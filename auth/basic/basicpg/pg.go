// Package basicpg содержит реализацию basic auth провайдера с использованием базы данных postgresql.
package basicpg

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"github.com/custom-app/sdk-go/auth"
	"github.com/custom-app/sdk-go/db/pg"
	"github.com/custom-app/sdk-go/service/workerpool/workerpoolpg"
	"github.com/custom-app/sdk-go/structs"
	"github.com/jackc/pgx/v4"
	"google.golang.org/protobuf/proto"
	"time"
)

// RoleQuery - sql-команда для получения из бд айди аккаунта по логину и паролю
type RoleQuery struct {
	Query string       // команда
	Role  structs.Role // роль для команды
}

// AuthorizationMaker - структура, имплементирующая интерфейс провайдера
type AuthorizationMaker struct {
	roleQueries   []RoleQuery
	authTimeout   time.Duration
	accountLoader func(ctx context.Context, tx *pg.Transaction, acc *structs.Account) proto.Message
	queue         *workerpoolpg.Queue
}

// NewMaker - создание AuthorizationMaker. roleQueries используется для перебора по логин/паролю, queue - очередь
// для контроля потока запросов, loader - функция получения полного авторизационного ответа по аккаунту,
// authTimeout - таймаут одной операции авторизации
func NewMaker(roleQueries []RoleQuery, queue *workerpoolpg.Queue,
	loader func(ctx context.Context, tx *pg.Transaction, acc *structs.Account) proto.Message,
	authTimeout time.Duration) *AuthorizationMaker {
	res := &AuthorizationMaker{
		roleQueries:   make([]RoleQuery, len(roleQueries)),
		queue:         queue,
		authTimeout:   authTimeout,
		accountLoader: loader,
	}
	copy(res.roleQueries, roleQueries)
	return res
}

// Auth - реализация метода Auth интерфейса AuthProvider
func (m *AuthorizationMaker) Auth(ctx context.Context, login, password string, platform structs.Platform,
	versions []string, disabled ...structs.Role) (*structs.Account, error) {
	var acc *structs.Account
	if err := m.queue.MakeJob(&workerpoolpg.Task{
		Ctx:          ctx,
		Options:      pgx.TxOptions{},
		QueueTimeout: time.Second,
		Timeout:      m.authTimeout,
		Worker: func(ctx context.Context, tx *pg.Transaction) (bool, *workerpoolpg.JobResultErr) {
			var err error
			acc, err = m.AuthWithTx(ctx, tx, login, password, platform, versions, disabled...)
			if err != nil {
				return false, workerpoolpg.WrapError(err)
			}
			return false, nil
		},
	}); err != nil {
		return nil, err
	}
	return acc, nil
}

// AuthWithTx - вспомогательная функция для авторизации с открытой бд-транзакцией
func (m *AuthorizationMaker) AuthWithTx(ctx context.Context, tx *pg.Transaction, login, password string,
	platform structs.Platform, versions []string, disabled ...structs.Role) (*structs.Account, error) {
	acc, err := m.findUserForLoginPass(ctx, tx, login, password, disabled...)
	if err != nil {
		return nil, err
	}
	acc.Platform, acc.Versions = platform, versions
	return acc, nil
}

// AuthWithInfo - реализация метода AuthWithInfo интерфейса AuthProvider
func (m *AuthorizationMaker) AuthWithInfo(ctx context.Context, login, password string, platform structs.Platform,
	versions []string, disabled ...structs.Role) (*structs.Account, proto.Message, error) {
	var (
		acc  *structs.Account
		resp proto.Message
	)
	if err := m.queue.MakeJob(&workerpoolpg.Task{
		Ctx:          ctx,
		Options:      pgx.TxOptions{},
		QueueTimeout: time.Second,
		Timeout:      m.authTimeout,
		Worker: func(ctx context.Context, tx *pg.Transaction) (bool, *workerpoolpg.JobResultErr) {
			var err error
			acc, err = m.findUserForLoginPass(ctx, tx, login, password, disabled...)
			if err != nil {
				return false, workerpoolpg.WrapError(err)
			}
			resp = m.accountLoader(ctx, tx, acc)
			return false, nil
		},
	}); err != nil {
		return nil, nil, err
	}
	acc.Platform, acc.Versions = platform, versions
	return acc, resp, nil
}

// Logout - реализация метода Logout интерфейса AuthProvider
func (m *AuthorizationMaker) Logout(ctx context.Context, role structs.Role, id int64) error {
	return nil
}

func hashedPassword(password string) (string, error) {
	pass, err := hex.DecodeString(password)
	if err != nil {
		return "", err
	}
	toCheck := sha256.Sum256(pass)
	return hex.EncodeToString(toCheck[:]), nil
}

func (m *AuthorizationMaker) findUserForLoginPass(ctx context.Context, tx *pg.Transaction,
	login, password string, disabled ...structs.Role) (*structs.Account, error) {
	pass, err := hashedPassword(password)
	if err != nil {
		return nil, err
	}
	for _, query := range m.roleQueries {
		var isDisabled bool
		for _, disabledRole := range disabled {
			if disabledRole == query.Role {
				isDisabled = true
				break
			}
		}
		if isDisabled {
			continue
		}
		getReq := tx.NewRequest(query.Query, login, pass)
		if err := getReq.Query(ctx); err != nil {
			return nil, err
		}
		if getReq.IsEmpty() {
			getReq.Close()
			continue
		}
		acc := &structs.Account{
			Role: query.Role,
		}
		if err := getReq.Scan(&acc.Id); err != nil {
			getReq.Close()
			return nil, err
		}
		getReq.Close()
		return acc, nil
	}
	return nil, auth.FailedAuthErr
}
