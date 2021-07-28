package pg

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"github.com/jackc/pgx/v4"
	"github.com/loyal-inform/sdk-go/auth"
	pg2 "github.com/loyal-inform/sdk-go/auth/pg"
	"github.com/loyal-inform/sdk-go/db/pg"
	"github.com/loyal-inform/sdk-go/service/job"
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
	queue         *job.DatabaseQueue
	authTimeout   time.Duration
	accountLoader pg2.AccountLoader
}

func NewMaker(roleQueries []RoleQuery, queue *job.DatabaseQueue, loader pg2.AccountLoader,
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
	if err := m.queue.MakeJob(ctx, pgx.TxOptions{}, func(ctx context.Context, tx *pg.Transaction) error {
		var err error
		acc, err = m.findUserForLoginPass(tx, login, password, disabled...)
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

func (m *AuthorizationMaker) AuthWithInfo(ctx context.Context, login, password string, platform structs.Platform,
	versions []string, disabled ...structs.Role) (*structs.Account, proto.Message, error) {
	var (
		acc  *structs.Account
		resp proto.Message
	)
	if err := m.queue.MakeJob(ctx, pgx.TxOptions{}, func(ctx context.Context, tx *pg.Transaction) error {
		var err error
		acc, err = m.findUserForLoginPass(tx, login, password, disabled...)
		if err != nil {
			return err
		}
		resp, err = m.accountLoader(tx, acc)
		return err
	}, m.authTimeout); err != nil {
		return nil, nil, err
	}
	acc.Platform, acc.Versions = platform, versions
	return acc, resp, nil
}

func HashedPassword(password string) (string, error) {
	pass, err := hex.DecodeString(password)
	if err != nil {
		return "", err
	}
	toCheck := sha256.Sum256(pass)
	return hex.EncodeToString(toCheck[:]), nil
}

func (m *AuthorizationMaker) findUserForLoginPass(tx *pg.Transaction, login, password string, disabled ...structs.Role) (*structs.Account, error) {
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
		if err := getReq.Query(); err != nil {
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
