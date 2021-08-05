package pg

import (
	"context"
	"github.com/jackc/pgx/v4"
	"github.com/loyal-inform/sdk-go/db/pg"
	"github.com/loyal-inform/sdk-go/structs"
	"google.golang.org/protobuf/proto"
	"time"
)

type DatabaseWorker func(ctx context.Context, tx *pg.Transaction) error

type DatabaseWorkerWithResponse func(ctx context.Context, tx *pg.Transaction) proto.Message

type DatabaseWorkerWithResult func(ctx context.Context, tx *pg.Transaction) structs.Result

func MakeJob(ctx context.Context, options pgx.TxOptions, worker DatabaseWorker,
	timeout time.Duration) error {
	var cancel func()
	ctx, cancel = context.WithTimeout(ctx, timeout)
	defer cancel()
	tx, err := pg.NewTransaction(ctx, options)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	resCh := make(chan error)
	go func() {
		resCh <- worker(ctx, tx)
	}()
	var res error
	select {
	case <-ctx.Done():
		tx.Rollback(ctx)
		<-resCh
		res = TimeoutErr
	case res = <-resCh:
		break
	}
	return res
}

func MakeJobWithResponse(ctx context.Context, options pgx.TxOptions, worker DatabaseWorkerWithResponse,
	timeout time.Duration) (proto.Message, error) {
	var cancel func()
	ctx, cancel = context.WithTimeout(ctx, timeout)
	defer cancel()
	tx, err := pg.NewTransaction(ctx, options)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	resCh := make(chan proto.Message)
	go func() {
		resCh <- worker(ctx, tx)
	}()
	var res proto.Message
	select {
	case <-ctx.Done():
		tx.Rollback(ctx)
		res = <-resCh
		return nil, TimeoutErr
	case res = <-resCh:
		break
	}
	return res, nil
}

func MakeJobWithResult(ctx context.Context, options pgx.TxOptions, worker DatabaseWorkerWithResult,
	timeout time.Duration) (structs.Result, error) {
	var cancel func()
	ctx, cancel = context.WithTimeout(ctx, timeout)
	defer cancel()
	tx, err := pg.NewTransaction(ctx, options)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	resCh := make(chan structs.Result)
	go func() {
		resCh <- worker(ctx, tx)
	}()
	var res structs.Result
	select {
	case <-ctx.Done():
		tx.Rollback(ctx)
		res = <-resCh
		return nil, TimeoutErr
	case res = <-resCh:
		break
	}
	return res, nil
}
