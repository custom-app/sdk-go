package pg

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/dgrijalva/jwt-go"
	"github.com/jackc/pgx/v4"
	"github.com/loyal-inform/sdk-go/auth"
	jwt2 "github.com/loyal-inform/sdk-go/auth/jwt"
	pg2 "github.com/loyal-inform/sdk-go/auth/pg"
	"github.com/loyal-inform/sdk-go/db/pg"
	pg3 "github.com/loyal-inform/sdk-go/service/job/pg"
	"github.com/loyal-inform/sdk-go/structs"
	"github.com/loyal-inform/sdk-go/util/locker"
	time2 "github.com/loyal-inform/sdk-go/util/time"
	"google.golang.org/protobuf/proto"
	"math/rand"
	"strings"
	"time"
)

const (
	alphabet = "abcdefghijklmnopqrstuvwxyz1234567890"
)

type AuthorizationMaker struct {
	tokenTables                                          map[structs.Role]string
	lockers                                              map[structs.Role]*locker.LockSystem
	key                                                  string
	queue                                                *pg3.Queue
	accessTokenTimeout, refreshTokenTimeout, authTimeout time.Duration
	accountLoader                                        pg2.AccountLoader
}

func init() {
	rand.Seed(time.Now().UnixNano())
}

func randomString(l int) string {
	res := make([]byte, l)
	for i := 0; i < l; i++ {
		res[i] = alphabet[rand.Intn(len(alphabet))]
	}
	return string(res)
}

func NewMaker(tokenTables map[structs.Role]string, key string, queue *pg3.Queue, loader pg2.AccountLoader,
	accessTokenTimeout, refreshTokenTimeout, authTimeout time.Duration) *AuthorizationMaker {
	res := &AuthorizationMaker{
		lockers:             make(map[structs.Role]*locker.LockSystem, len(tokenTables)),
		tokenTables:         make(map[structs.Role]string, len(tokenTables)),
		key:                 key,
		queue:               queue,
		accountLoader:       loader,
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
	platform structs.Platform, versions []string, disabled ...structs.Role) (*structs.Account, int64, error) {
	t, err := m.parseToken(token)
	if err != nil {
		return nil, 0, err
	}
	var (
		acc    *structs.Account
		number int64
	)
	if err := m.queue.MakeJob(&pg3.Task{
		Ctx:          ctx,
		Options:      pgx.TxOptions{},
		QueueTimeout: time.Second,
		Timeout:      m.authTimeout,
		Worker: func(ctx context.Context, tx *pg.Transaction) (bool, *pg3.JobResultErr) {
			acc, number, err = m.checkToken(ctx, tx, t, purpose)
			if err != nil {
				return false, pg3.WrapError(err)
			}
			for _, r := range disabled {
				if r == acc.Role {
					return false, pg3.WrapError(auth.PermissionDeniedErr)
				}
			}
			return false, nil
		},
	}); err != nil {
		return nil, 0, err
	}
	acc.Platform, acc.Versions = platform, versions
	return acc, number, nil
}

func (m *AuthorizationMaker) AuthWithInfo(ctx context.Context, token string, purpose structs.Purpose, platform structs.Platform,
	versions []string, disabled ...structs.Role) (*structs.Account, int64, proto.Message, error) {
	t, err := m.parseToken(token)
	if err != nil {
		return nil, 0, nil, err
	}
	var (
		acc    *structs.Account
		resp   proto.Message
		number int64
	)
	if err := m.queue.MakeJob(&pg3.Task{
		Ctx:          ctx,
		Options:      pgx.TxOptions{},
		QueueTimeout: time.Second,
		Timeout:      m.authTimeout,
		Worker: func(ctx context.Context, tx *pg.Transaction) (bool, *pg3.JobResultErr) {
			acc, number, err = m.checkToken(ctx, tx, t, purpose)
			if err != nil {
				return false, pg3.WrapError(err)
			}
			for _, r := range disabled {
				if r == acc.Role {
					return false, pg3.WrapError(auth.PermissionDeniedErr)
				}
			}
			resp = m.accountLoader(ctx, tx, acc)
			return false, nil
		},
	}); err != nil {
		return nil, 0, nil, err
	}
	acc.Platform, acc.Versions = platform, versions
	return acc, number, resp, nil
}

func (m *AuthorizationMaker) Logout(ctx context.Context, role structs.Role, id int64) error {
	return m.queue.MakeJob(&pg3.Task{
		Ctx:          ctx,
		Options:      pgx.TxOptions{},
		QueueTimeout: time.Second,
		Timeout:      m.authTimeout,
		Worker: func(ctx context.Context, tx *pg.Transaction) (bool, *pg3.JobResultErr) {
			if err := m.DropAllTokens(ctx, tx, role, id); err != nil {
				return false, pg3.WrapError(err)
			}
			return true, nil
		},
	})
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

func (m *AuthorizationMaker) checkToken(ctx context.Context, tx *pg.Transaction,
	token *jwt.Token, purpose structs.Purpose) (*structs.Account, int64, error) {
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, 0, jwt2.ParseTokenErr
	}
	if !claims.VerifyExpiresAt(time2.Now().Unix(), true) {
		return nil, 0, jwt2.ExpiredTokenErr
	}
	if realPurpose, ok := claims["purpose"].(float64); !ok {
		return nil, 0, jwt2.InvalidTokenErr
	} else if structs.Purpose(realPurpose) != purpose {
		return nil, 0, jwt2.InvalidTokenPurposeErr
	}
	if !token.Valid {
		return nil, 0, jwt2.InvalidTokenErr
	}
	id, err := parseTokenIntClaim(claims, "id")
	if err != nil {
		return nil, 0, err
	}
	number, err := parseTokenIntClaim(claims, "number")
	if err != nil {
		return nil, 0, err
	}
	role, err := parseTokenIntClaim(claims, "role")
	if err != nil {
		return nil, 0, err
	}
	secret, err := parseTokenStringClaim(claims, "secret")
	if err != nil {
		return nil, 0, err
	}
	if _, ok := m.tokenTables[structs.Role(role)]; !ok {
		return nil, 0, jwt2.InvalidTokenErr
	}
	if e := m.checkSecret(ctx, tx, structs.Role(role), id, number, purpose, secret); e != nil {
		return nil, 0, e
	}
	return &structs.Account{
		Role: structs.Role(role),
		Id:   id,
	}, number, nil
}

func parseTokenIntClaim(claims jwt.MapClaims, key string) (int64, error) {
	if parsedValue, ok := claims[key].(float64); !ok {
		return 0, jwt2.InvalidTokenErr
	} else {
		return int64(parsedValue), nil
	}
}

func parseTokenStringClaim(claims jwt.MapClaims, key string) (string, error) {
	if stringValue, ok := claims[key].(string); !ok {
		return "", jwt2.InvalidTokenErr
	} else {
		return stringValue, nil
	}
}

func (m *AuthorizationMaker) checkSecret(ctx context.Context, tx *pg.Transaction, role structs.Role,
	id, number int64, purpose structs.Purpose, secret string) error {
	checkReq := tx.NewRequest(fmt.Sprintf("select 1 from %s where id=$1 and number=$2 and purpose=$3 and secret=$4",
		m.tokenTables[role]), id, number, purpose, secret)
	if err := checkReq.Query(ctx); err != nil {
		return err
	}
	checkReq.Close()
	if checkReq.IsEmpty() {
		return auth.FailedAuthErr
	}
	return nil
}

func GenerateSecret(role structs.Role, id, number int64, purpose structs.Purpose) string {
	toHashElems := []string{
		fmt.Sprintf("%d", role),
		fmt.Sprintf("%d", id),
		fmt.Sprintf("%d", number),
		fmt.Sprintf("%d", purpose),
		fmt.Sprintf("%d", time2.Now().UnixNano()),
		randomString(20),
	}
	toHash := strings.Join(toHashElems, "_")
	hash := sha256.Sum256([]byte(toHash))
	return hex.EncodeToString(hash[:])
}

func (m *AuthorizationMaker) findNumber(ctx context.Context, tx *pg.Transaction, role structs.Role, id int64) (int64, error) {
	getReq := tx.NewRequest(fmt.Sprintf("select array(select number from %s where id=$1 and purpose=0 order by number)",
		m.tokenTables[role]), id)
	if err := getReq.Query(ctx); err != nil {
		return 0, err
	}
	defer getReq.Close()
	var numbers []int64
	if err := getReq.Scan(&numbers); err != nil {
		return 0, err
	}
	if len(numbers) == 0 {
		return 0, nil
	}
	if numbers[len(numbers)-1] == int64(len(numbers)-1) {
		return int64(len(numbers)), nil
	}
	for i, n := range numbers {
		if n != int64(i) {
			return int64(i), nil
		}
	}
	return 0, fmt.Errorf("number assignment failed")
}

func (m *AuthorizationMaker) setSecret(ctx context.Context, tx *pg.Transaction, role structs.Role,
	id, number int64, purpose structs.Purpose, secret string, expires time.Time) error {
	insertReq := tx.NewRequest(fmt.Sprintf("insert into %s values($1,$2,$3,$4,$5)", m.tokenTables[role]),
		id, number, purpose, secret, expires.UnixNano()/1e+6)
	if err := insertReq.Exec(ctx); err != nil {
		return err
	}
	return nil
}

func (m *AuthorizationMaker) createToken(ctx context.Context, tx *pg.Transaction, role structs.Role, id, number int64,
	purpose structs.Purpose, expire time.Time) (string, error) {
	secret := GenerateSecret(role, id, number, purpose)
	if e := m.setSecret(ctx, tx, role, id, number, purpose, secret, expire); e != nil {
		return "", e
	}
	claims := jwt.MapClaims{
		"id":      id,
		"role":    role,
		"purpose": purpose,
		"secret":  secret,
		"exp":     expire.Unix(),
		"number":  number,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	res, e := token.SignedString([]byte(m.key))
	if e != nil {
		return "", e
	}
	return res, nil
}

func (m *AuthorizationMaker) DropTokens(ctx context.Context, role structs.Role, id, number int64) error {
	return m.queue.MakeJob(&pg3.Task{
		Ctx:          ctx,
		Options:      pgx.TxOptions{},
		QueueTimeout: time.Second,
		Timeout:      m.authTimeout,
		Worker: func(ctx context.Context, tx *pg.Transaction) (bool, *pg3.JobResultErr) {
			m.lockers[role].Lock(id)
			defer m.lockers[role].Unlock(id)
			if err := m.dropTokens(ctx, tx, role, id, number); err != nil {
				return false, pg3.WrapError(err)
			}
			return true, nil
		},
	})
}

func (m *AuthorizationMaker) dropTokens(ctx context.Context, tx *pg.Transaction, role structs.Role, id, number int64) error {
	dropReq := tx.NewRequest(fmt.Sprintf("delete from %s where id=$1 and number=$2", m.tokenTables[role]), id, number)
	if err := dropReq.Exec(ctx); err != nil {
		return err
	}
	return nil
}

func (m *AuthorizationMaker) DropAllTokens(ctx context.Context, tx *pg.Transaction, role structs.Role, id int64) error {
	m.lockers[role].Lock(id)
	defer m.lockers[role].Unlock(id)
	dropReq := tx.NewRequest(fmt.Sprintf("delete from %s where id=$1", m.tokenTables[role]), id)
	if err := dropReq.Exec(ctx); err != nil {
		return err
	}
	return nil
}

func (m *AuthorizationMaker) ReCreateTokens(ctx context.Context, role structs.Role,
	id, number int64) (accessToken string, accessExpires int64, refreshToken string, refreshExpires int64, err error) {
	if err := m.queue.MakeJob(&pg3.Task{
		Ctx:          ctx,
		Options:      pgx.TxOptions{},
		QueueTimeout: time.Second,
		Timeout:      m.authTimeout,
		Worker: func(ctx context.Context, tx *pg.Transaction) (bool, *pg3.JobResultErr) {
			m.lockers[role].Lock(id)
			defer m.lockers[role].Unlock(id)
			if e := m.dropTokens(ctx, tx, role, id, number); e != nil {
				return false, pg3.WrapError(e)
			}
			accessToken, accessExpires, refreshToken, refreshExpires, err = m.createTokens(ctx, tx, role, id, number)
			if err != nil {
				return false, pg3.WrapError(err)
			}
			return true, nil
		},
	}); err != nil {
		return "", 0, "", 0, err
	}
	return
}

func (m *AuthorizationMaker) CreateTokens(ctx context.Context, role structs.Role,
	id int64) (accessToken string, accessExpires int64, refreshToken string, refreshExpires int64, err error) {
	if err := m.queue.MakeJob(&pg3.Task{
		Ctx:          ctx,
		Options:      pgx.TxOptions{},
		QueueTimeout: time.Second,
		Timeout:      m.authTimeout,
		Worker: func(ctx context.Context, tx *pg.Transaction) (bool, *pg3.JobResultErr) {
			accessToken, accessExpires, refreshToken, refreshExpires, err = m.CreateTokensWithTx(ctx, tx, role, id)
			if err != nil {
				return false, pg3.WrapError(err)
			}
			return true, nil
		},
	}); err != nil {
		return "", 0, "", 0, err
	}
	return
}

func (m *AuthorizationMaker) CreateTokensWithTx(ctx context.Context, tx *pg.Transaction, role structs.Role,
	id int64) (accessToken string, accessExpires int64, refreshToken string, refreshExpires int64, err error) {
	m.lockers[role].Lock(id)
	defer m.lockers[role].Unlock(id)
	number, e := m.findNumber(ctx, tx, role, id)
	if e != nil {
		return "", 0, "", 0, e
	}
	accessToken, accessExpires, refreshToken, refreshExpires, err = m.createTokens(ctx, tx, role, id, number)
	if err != nil {
		return "", 0, "", 0, err
	}
	return
}

func (m *AuthorizationMaker) createTokens(ctx context.Context, tx *pg.Transaction, role structs.Role,
	id, number int64) (string, int64, string, int64, error) {
	now := time.Now()
	accessExpiresAt, refreshExpiresAt := now.Add(m.accessTokenTimeout), now.Add(m.refreshTokenTimeout)
	accessToken, e := m.createToken(ctx, tx, role, id, number, structs.PurposeAccess, accessExpiresAt)
	if e != nil {
		return "", 0, "", 0, e
	}
	refreshToken, e := m.createToken(ctx, tx, role, id, number, structs.PurposeRefresh, refreshExpiresAt)
	if e != nil {
		return "", 0, "", 0, e
	}
	return accessToken, accessExpiresAt.UnixNano() / 1e+6, refreshToken, refreshExpiresAt.UnixNano() / 1e+6, nil
}

func (m *AuthorizationMaker) DropOldTokens(ctx context.Context, timestamp int64) error {
	return m.queue.MakeJob(&pg3.Task{
		Ctx:          ctx,
		Options:      pgx.TxOptions{},
		QueueTimeout: time.Second,
		Timeout:      m.authTimeout,
		Worker: func(ctx context.Context, tx *pg.Transaction) (bool, *pg3.JobResultErr) {
			for _, v := range m.tokenTables {
				dropReq := tx.NewRequest(fmt.Sprintf("delete from %s where number in "+
					"(select number from %s where purpose=$1 and expires_at<=$2)", v, v),
					structs.PurposeRefresh, timestamp)
				if err := dropReq.Exec(ctx); err != nil {
					return false, pg3.WrapError(err)
				}
			}
			return true, nil
		},
	})
}
