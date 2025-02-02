// Package jwtpg содержит реализацию jwt провайдера с одним единовременным токеном с использованием базы данных postgresql.
//
// Для работы с пакетом токены должны храниться в таблицах такого вида:
//
//     Column   |     Type      | Collation | Nullable | Default
//  ------------+---------------+-----------+----------+---------
//   id         | bigint        |           | not null |
//   number     | bigint        |           | not null |
//   purpose    | integer       |           | not null |
//   secret     | character(64) |           | not null |
//   expires_at | bigint        |           | not null |
package jwtpg

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/custom-app/sdk-go/auth"
	jwt2 "github.com/custom-app/sdk-go/auth/jwt"
	"github.com/custom-app/sdk-go/db/pg"
	"github.com/custom-app/sdk-go/service/workerpool/workerpoolpg"
	"github.com/custom-app/sdk-go/structs"
	"github.com/custom-app/sdk-go/util/locker"
	"github.com/dgrijalva/jwt-go"
	"github.com/jackc/pgx/v4"
	"google.golang.org/protobuf/proto"
	"math/rand"
	"strings"
	"time"
)

const (
	alphabet = "abcdefghijklmnopqrstuvwxyz1234567890"
)

// AuthorizationMaker - структура, имплементирующая интерфейс провайдера
type AuthorizationMaker struct {
	tokenTables                                          map[structs.Role]string
	lockers                                              map[structs.Role]*locker.LockSystem
	key                                                  string
	queue                                                *workerpoolpg.Queue
	accessTokenTimeout, refreshTokenTimeout, authTimeout time.Duration
	accountLoader                                        func(ctx context.Context, tx *pg.Transaction, acc *structs.Account) proto.Message
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

// NewMaker - создание AuthorizationMaker. tokenTables - названия таблиц с токенами, key - секретный ключ,
// queue - очередь для контроля потока запросов, loader - функция получения полного авторизационного ответа по аккаунту,
// accessTokenTimeout - время жизни access токена, refreshTokenTimeout - время жизни refresh токена,
// authTimeout - таймаут одной операции авторизации
func NewMaker(tokenTables map[structs.Role]string, key string, queue *workerpoolpg.Queue,
	loader func(ctx context.Context, tx *pg.Transaction, acc *structs.Account) proto.Message,
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

// Auth - реализация метода Auth интерфейса AuthProvider
func (m *AuthorizationMaker) Auth(ctx context.Context, token string, purpose jwt2.Purpose,
	platform structs.Platform, versions []string, disabled ...structs.Role) (*structs.Account, int64, error) {
	t, err := m.parseToken(token)
	if err != nil {
		return nil, 0, err
	}
	var (
		acc    *structs.Account
		number int64
	)
	if err := m.queue.MakeJob(&workerpoolpg.Task{
		Ctx:          ctx,
		Options:      pgx.TxOptions{},
		QueueTimeout: time.Second,
		Timeout:      m.authTimeout,
		Worker: func(ctx context.Context, tx *pg.Transaction) (bool, *workerpoolpg.JobResultErr) {
			acc, number, err = m.checkToken(ctx, tx, t, purpose)
			if err != nil {
				return false, workerpoolpg.WrapError(err)
			}
			for _, r := range disabled {
				if r == acc.Role {
					return false, workerpoolpg.WrapError(auth.PermissionDeniedErr)
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

// AuthWithInfo - реализация метода AuthWithInfo интерфейса AuthProvider
func (m *AuthorizationMaker) AuthWithInfo(ctx context.Context, token string, purpose jwt2.Purpose, platform structs.Platform,
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
	if err := m.queue.MakeJob(&workerpoolpg.Task{
		Ctx:          ctx,
		Options:      pgx.TxOptions{},
		QueueTimeout: time.Second,
		Timeout:      m.authTimeout,
		Worker: func(ctx context.Context, tx *pg.Transaction) (bool, *workerpoolpg.JobResultErr) {
			acc, number, err = m.checkToken(ctx, tx, t, purpose)
			if err != nil {
				return false, workerpoolpg.WrapError(err)
			}
			for _, r := range disabled {
				if r == acc.Role {
					return false, workerpoolpg.WrapError(auth.PermissionDeniedErr)
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

// Logout - реализация метода Logout интерфейса AuthProvider
func (m *AuthorizationMaker) Logout(ctx context.Context, role structs.Role, id int64) error {
	return m.queue.MakeJob(&workerpoolpg.Task{
		Ctx:          ctx,
		Options:      pgx.TxOptions{},
		QueueTimeout: time.Second,
		Timeout:      m.authTimeout,
		Worker: func(ctx context.Context, tx *pg.Transaction) (bool, *workerpoolpg.JobResultErr) {
			if err := m.DropAllTokens(ctx, tx, role, id); err != nil {
				return false, workerpoolpg.WrapError(err)
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
	token *jwt.Token, purpose jwt2.Purpose) (*structs.Account, int64, error) {
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, 0, jwt2.ParseTokenErr
	}
	if !claims.VerifyExpiresAt(time.Now().Unix(), true) {
		return nil, 0, jwt2.ExpiredTokenErr
	}
	if realPurpose, ok := claims["purpose"].(float64); !ok {
		return nil, 0, jwt2.InvalidTokenErr
	} else if jwt2.Purpose(realPurpose) != purpose {
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
	id, number int64, purpose jwt2.Purpose, secret string) error {
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

func generateSecret(role structs.Role, id, number int64, purpose jwt2.Purpose) string {
	toHashElems := []string{
		fmt.Sprintf("%d", role),
		fmt.Sprintf("%d", id),
		fmt.Sprintf("%d", number),
		fmt.Sprintf("%d", purpose),
		fmt.Sprintf("%d", time.Now().UnixNano()),
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
	id, number int64, purpose jwt2.Purpose, secret string, expires time.Time) error {
	insertReq := tx.NewRequest(fmt.Sprintf("insert into %s values($1,$2,$3,$4,$5)", m.tokenTables[role]),
		id, number, purpose, secret, expires.UnixNano()/1e+6)
	if err := insertReq.Exec(ctx); err != nil {
		return err
	}
	return nil
}

func (m *AuthorizationMaker) createToken(ctx context.Context, tx *pg.Transaction, role structs.Role, id, number int64,
	purpose jwt2.Purpose, expire time.Time) (string, error) {
	secret := generateSecret(role, id, number, purpose)
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

// DropTokens - реализация метода DropTokens интерфейса AuthProvider
func (m *AuthorizationMaker) DropTokens(ctx context.Context, role structs.Role, id, number int64) error {
	m.lockers[role].Lock(id)
	defer m.lockers[role].Unlock(id)
	return m.queue.MakeJob(&workerpoolpg.Task{
		Ctx:          ctx,
		Options:      pgx.TxOptions{},
		QueueTimeout: time.Second,
		Timeout:      m.authTimeout,
		Worker: func(ctx context.Context, tx *pg.Transaction) (bool, *workerpoolpg.JobResultErr) {
			if err := m.dropTokens(ctx, tx, role, id, number); err != nil {
				return false, workerpoolpg.WrapError(err)
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

// DropAllTokens - функция удаления всех токенов аккаунта
func (m *AuthorizationMaker) DropAllTokens(ctx context.Context, tx *pg.Transaction, role structs.Role, id int64) error {
	m.lockers[role].Lock(id)
	defer m.lockers[role].Unlock(id)
	dropReq := tx.NewRequest(fmt.Sprintf("delete from %s where id=$1", m.tokenTables[role]), id)
	if err := dropReq.Exec(ctx); err != nil {
		return err
	}
	return nil
}

// ReCreateTokens - реализация метода ReCreateTokens интерфейса AuthProvider
func (m *AuthorizationMaker) ReCreateTokens(ctx context.Context, role structs.Role,
	id, number int64) (accessToken string, accessExpires int64, refreshToken string, refreshExpires int64, err error) {
	m.lockers[role].Lock(id)
	defer m.lockers[role].Unlock(id)
	if err := m.queue.MakeJob(&workerpoolpg.Task{
		Ctx:          ctx,
		Options:      pgx.TxOptions{},
		QueueTimeout: time.Second,
		Timeout:      m.authTimeout,
		Worker: func(ctx context.Context, tx *pg.Transaction) (bool, *workerpoolpg.JobResultErr) {
			if e := m.dropTokens(ctx, tx, role, id, number); e != nil {
				return false, workerpoolpg.WrapError(e)
			}
			accessToken, accessExpires, refreshToken, refreshExpires, err = m.createTokens(ctx, tx, role, id, number)
			if err != nil {
				return false, workerpoolpg.WrapError(err)
			}
			return true, nil
		},
	}); err != nil {
		return "", 0, "", 0, err
	}
	return
}

// CreateTokens - реализация метода CreateTokens интерфейса AuthProvider
func (m *AuthorizationMaker) CreateTokens(ctx context.Context, role structs.Role,
	id int64) (accessToken string, accessExpires int64, refreshToken string, refreshExpires int64, err error) {
	if err := m.queue.MakeJob(&workerpoolpg.Task{
		Ctx:          ctx,
		Options:      pgx.TxOptions{},
		QueueTimeout: time.Second,
		Timeout:      m.authTimeout,
		Worker: func(ctx context.Context, tx *pg.Transaction) (bool, *workerpoolpg.JobResultErr) {
			accessToken, accessExpires, refreshToken, refreshExpires, err = m.CreateTokensWithTx(ctx, tx, role, id)
			if err != nil {
				return false, workerpoolpg.WrapError(err)
			}
			return true, nil
		},
	}); err != nil {
		return "", 0, "", 0, err
	}
	return
}

// CreateTokensWithTx - вспомогательная функция создания токенов с открытой бд-транзакцией
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
	accessToken, e := m.createToken(ctx, tx, role, id, number, jwt2.PurposeAccess, accessExpiresAt)
	if e != nil {
		return "", 0, "", 0, e
	}
	refreshToken, e := m.createToken(ctx, tx, role, id, number, jwt2.PurposeRefresh, refreshExpiresAt)
	if e != nil {
		return "", 0, "", 0, e
	}
	return accessToken, accessExpiresAt.UnixNano() / 1e+6, refreshToken, refreshExpiresAt.UnixNano() / 1e+6, nil
}

// DropOldTokens - функция удаления устаревших токенов
func (m *AuthorizationMaker) DropOldTokens(ctx context.Context, timestamp int64) error {
	return m.queue.MakeJob(&workerpoolpg.Task{
		Ctx:          ctx,
		Options:      pgx.TxOptions{},
		QueueTimeout: time.Second,
		Timeout:      m.authTimeout,
		Worker: func(ctx context.Context, tx *pg.Transaction) (bool, *workerpoolpg.JobResultErr) {
			for _, v := range m.tokenTables {
				dropReq := tx.NewRequest(fmt.Sprintf("delete from %s where number in "+
					"(select number from %s where purpose=$1 and expires_at<=$2)", v, v),
					jwt2.PurposeRefresh, timestamp)
				if err := dropReq.Exec(ctx); err != nil {
					return false, workerpoolpg.WrapError(err)
				}
			}
			return true, nil
		},
	})
}
