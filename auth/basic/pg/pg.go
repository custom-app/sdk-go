package pg

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"github.com/jackc/pgx/v4"
	"github.com/loyal-inform/sdk-go/auth"
	pg2 "github.com/loyal-inform/sdk-go/auth/pg"
	"github.com/loyal-inform/sdk-go/db/pg"
	pg3 "github.com/loyal-inform/sdk-go/service/job/pg"
	"github.com/loyal-inform/sdk-go/structs"
	"google.golang.org/protobuf/proto"
	"time"
)

type RoleQuery struct {
	Query string
	Role  structs.Role
}

type AuthorizationMaker struct {
	roleQueries   []RoleQuery
	authTimeout   time.Duration
	accountLoader pg2.AccountLoader
	queue         *pg3.Queue
}

func NewMaker(roleQueries []RoleQuery, queue *pg3.Queue, loader pg2.AccountLoader,
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

func (m *AuthorizationMaker) Auth(ctx context.Context, login, password string, platform structs.Platform,
	versions []string, disabled ...structs.Role) (*structs.Account, error) {
	var acc *structs.Account
	if err := m.queue.MakeJob(&pg3.Task{
		Ctx:          ctx,
		Options:      pgx.TxOptions{},
		QueueTimeout: time.Second,
		Timeout:      m.authTimeout,
		Worker: func(ctx context.Context, tx *pg.Transaction) (bool, []func(), error) {
			var err error
			acc, err = m.AuthWithTx(ctx, tx, login, password, platform, versions, disabled...)
			if err != nil {
				return false, nil, err
			}
			return false, nil, nil
		},
	}); err != nil {
		return nil, err
	}
	return acc, nil
}

func (m *AuthorizationMaker) AuthWithTx(ctx context.Context, tx *pg.Transaction, login, password string,
	platform structs.Platform, versions []string, disabled ...structs.Role) (*structs.Account, error) {
	acc, err := m.findUserForLoginPass(ctx, tx, login, password, disabled...)
	if err != nil {
		return nil, err
	}
	acc.Platform, acc.Versions = platform, versions
	return acc, nil
}

func (m *AuthorizationMaker) AuthWithInfo(ctx context.Context, login, password string, platform structs.Platform,
	versions []string, disabled ...structs.Role) (*structs.Account, proto.Message, error) {
	var (
		acc  *structs.Account
		resp proto.Message
	)
	if err := m.queue.MakeJob(&pg3.Task{
		Ctx:          ctx,
		Options:      pgx.TxOptions{},
		QueueTimeout: time.Second,
		Timeout:      m.authTimeout,
		Worker: func(ctx context.Context, tx *pg.Transaction) (bool, []func(), error) {
			var err error
			acc, err = m.findUserForLoginPass(ctx, tx, login, password, disabled...)
			if err != nil {
				return false, nil, err
			}
			resp = m.accountLoader(ctx, tx, acc)
			return false, nil, nil
		},
	}); err != nil {
		return nil, nil, err
	}
	acc.Platform, acc.Versions = platform, versions
	return acc, resp, nil
}

func (m *AuthorizationMaker) Logout(ctx context.Context, role structs.Role, id int64) error {
	return nil
}

func HashedPassword(password string) (string, error) {
	pass, err := hex.DecodeString(password)
	if err != nil {
		return "", err
	}
	toCheck := sha256.Sum256(pass)
	return hex.EncodeToString(toCheck[:]), nil
}

func (m *AuthorizationMaker) findUserForLoginPass(ctx context.Context, tx *pg.Transaction,
	login, password string, disabled ...structs.Role) (*structs.Account, error) {
	pass, err := HashedPassword(password)
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
