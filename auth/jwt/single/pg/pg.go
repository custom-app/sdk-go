package pg

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/dgrijalva/jwt-go"
	"github.com/jackc/pgx/v4"
	jwt2 "github.com/loyal-inform/sdk-go/auth/jwt"
	"github.com/loyal-inform/sdk-go/db/pg"
	"github.com/loyal-inform/sdk-go/service/job"
	"github.com/loyal-inform/sdk-go/structs"
	"github.com/loyal-inform/sdk-go/util/locker"
	time2 "github.com/loyal-inform/sdk-go/util/time"
	"math/rand"
	"strings"
	"time"
)

const (
	alphabet = "abcdefghijklmnopqrstuvwxyz1234567890"
)

func init() {
	rand.Seed(time2.Now().UnixNano())
}

func randomString(l int) string {
	res := make([]byte, l)
	for i := 0; i < l; i++ {
		res[i] = alphabet[rand.Intn(len(alphabet))]
	}
	return string(res)
}

type AuthorizationMaker struct {
	tokenTables                                          map[structs.Role]string
	lockers                                              map[structs.Role]*locker.LockSystem
	key                                                  string
	queue                                                *job.DatabaseQueue
	accessTokenTimeout, refreshTokenTimeout, authTimeout time.Duration
}

func NewMaker(tokenTables map[structs.Role]string, key string, queue *job.DatabaseQueue,
	accessTokenTimeout, refreshTokenTimeout, authTimeout time.Duration) *AuthorizationMaker {
	res := &AuthorizationMaker{
		lockers:             make(map[structs.Role]*locker.LockSystem, len(tokenTables)),
		tokenTables:         make(map[structs.Role]string, len(tokenTables)),
		key:                 key,
		queue:               queue,
		accessTokenTimeout:  accessTokenTimeout,
		refreshTokenTimeout: refreshTokenTimeout,
		authTimeout:         authTimeout,
	}
	for k, v := range tokenTables {
		res.tokenTables[k] = v
		res.lockers[k] = locker.NewLockSystem()
	}
	return res
}

func (m *AuthorizationMaker) Auth(ctx context.Context, token string, purpose structs.Purpose,
	platform structs.Platform, versions []string) (*structs.Account, error) {
	t, err := m.parseToken(token)
	if err != nil {
		return nil, err
	}
	var acc *structs.Account
	if err := m.queue.MakeJob(ctx, pgx.TxOptions{}, func(ctx context.Context, tx *pg.Transaction) error {
		acc, err = m.checkToken(tx, t, purpose)
		if err != nil {
			return err
		}
		return nil
	}, m.authTimeout); err != nil {
		return nil, err
	}
	acc.Platform, acc.Versions = platform, versions
	return acc, nil
}

func (m *AuthorizationMaker) parseToken(token string) (*jwt.Token, error) {
	res, err := jwt.Parse(token, func(token *jwt.Token) (i interface{}, e error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(m.key), nil
	})
	if err != nil && res == nil {
		return nil, jwt2.ParseTokenErr
	}
	return res, nil
}

func (m *AuthorizationMaker) checkToken(tx *pg.Transaction, token *jwt.Token, purpose structs.Purpose) (*structs.Account, error) {
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, jwt2.ParseTokenErr
	}
	if !claims.VerifyExpiresAt(time2.Now().Unix(), true) {
		return nil, jwt2.ExpiredToken
	}
	if realPurpose, ok := claims["purpose"].(float64); !ok {
		return nil, jwt2.InvalidToken
	} else if structs.Purpose(realPurpose) != purpose {
		return nil, jwt2.InvalidTokenPurpose
	}
	if !token.Valid {
		return nil, jwt2.InvalidToken
	}
	id, err := parseTokenIntClaim(claims, "id")
	if err != nil {
		return nil, err
	}
	role, err := parseTokenIntClaim(claims, "role")
	if err != nil {
		return nil, err
	}
	secret, err := parseTokenStringClaim(claims, "secret")
	if err != nil {
		return nil, err
	}
	if _, ok := m.tokenTables[structs.Role(role)]; !ok {
		return nil, jwt2.InvalidToken
	}
	if e := m.checkSecret(tx, structs.Role(role), id, purpose, secret); e != nil {
		return nil, e
	}
	return &structs.Account{
		Role: structs.Role(role),
		Id:   id,
	}, nil
}

func parseTokenIntClaim(claims jwt.MapClaims, key string) (int64, error) {
	if parsedValue, ok := claims[key].(float64); !ok {
		return 0, jwt2.InvalidToken
	} else {
		return int64(parsedValue), nil
	}
}

func parseTokenStringClaim(claims jwt.MapClaims, key string) (string, error) {
	if stringValue, ok := claims[key].(string); !ok {
		return "", jwt2.InvalidToken
	} else {
		return stringValue, nil
	}
}

func (m *AuthorizationMaker) checkSecret(tx *pg.Transaction, role structs.Role, id int64, purpose structs.Purpose, secret string) error {
	checkReq := tx.NewRequest(fmt.Sprintf("select 1 from %s where id=$1 and purpose=$2 and secret=$3",
		m.tokenTables[role]), id, purpose, secret)
	if err := checkReq.Query(); err != nil {
		return err
	}
	checkReq.Close()
	if checkReq.IsEmpty() {
		return jwt2.AuthFailedErr
	}
	return nil
}

func GenerateSecret(role structs.Role, id int64, purpose structs.Purpose) string {
	toHashElems := []string{
		fmt.Sprintf("%d", role),
		fmt.Sprintf("%d", id),
		fmt.Sprintf("%d", purpose),
		fmt.Sprintf("%d", time2.Now().UnixNano()),
		randomString(20),
	}
	toHash := strings.Join(toHashElems, "_")
	hash := sha256.Sum256([]byte(toHash))
	return hex.EncodeToString(hash[:])
}

func (m *AuthorizationMaker) setSecret(tx *pg.Transaction, role structs.Role, id int64, purpose structs.Purpose, secret string, expires time.Time) error {
	dropReq := tx.NewRequest(fmt.Sprintf("delete from %s where id=$1 and purpose=$2", m.tokenTables[role]), id, purpose)
	if err := dropReq.Exec(); err != nil {
		return err
	}
	insertReq := tx.NewRequest(fmt.Sprintf("insert into %s values($1,$2,$3,$4)", m.tokenTables[role]),
		id, purpose, secret, expires.UnixNano()/1e+6)
	if err := insertReq.Exec(); err != nil {
		return err
	}
	return nil
}

func (m *AuthorizationMaker) createToken(tx *pg.Transaction, role structs.Role, id int64,
	purpose structs.Purpose, expire time.Time) (string, error) {
	secret := GenerateSecret(role, id, purpose)
	if e := m.setSecret(tx, role, id, purpose, secret, expire); e != nil {
		return "", e
	}
	claims := jwt.MapClaims{
		"id":      id,
		"role":    role,
		"purpose": purpose,
		"secret":  secret,
		"exp":     expire.Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	res, e := token.SignedString([]byte(m.key))
	if e != nil {
		return "", e
	}
	return res, nil
}

func (m *AuthorizationMaker) CreateTokens(tx *pg.Transaction, role structs.Role,
	id int64) (string, int64, string, int64, error) {
	m.lockers[role].Lock(id)
	defer m.lockers[role].Unlock(id)
	return m.createTokens(tx, role, id)
}

func (m *AuthorizationMaker) createTokens(tx *pg.Transaction, role structs.Role,
	id int64) (string, int64, string, int64, error) {
	now := time.Now()
	accessExpiresAt, refreshExpiresAt := now.Add(m.accessTokenTimeout), now.Add(m.refreshTokenTimeout)
	accessToken, e := m.createToken(tx, role, id, structs.PurposeAccess, accessExpiresAt)
	if e != nil {
		return "", 0, "", 0, e
	}
	refreshToken, e := m.createToken(tx, role, id, structs.PurposeRefresh, refreshExpiresAt)
	if e != nil {
		return "", 0, "", 0, e
	}
	return accessToken, accessExpiresAt.UnixNano() / 1e+6, refreshToken, refreshExpiresAt.UnixNano() / 1e+6, nil
}
