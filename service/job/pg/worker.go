package pg

import (
	"context"
	"github.com/jackc/pgx/v4"
	"github.com/loyal-inform/sdk-go/db/pg"
	"github.com/loyal-inform/sdk-go/logger"
	"github.com/loyal-inform/sdk-go/structs"
	"google.golang.org/protobuf/proto"
	"time"
)

const rollbackTimeout = 2 * time.Second

type DatabaseWorker func(ctx context.Context, tx *pg.Transaction) error

type DatabaseWorkerWithResponse func(ctx context.Context, tx *pg.Transaction) proto.Message

type DatabaseWorkerWithResult func(ctx context.Context, tx *pg.Transaction) structs.Result

func rollbackWithTimeout(tx *pg.Transaction, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	tx.Rollback(ctx)
}

func MakeJob(ctx context.Context, options pgx.TxOptions, worker DatabaseWorker,
	timeout time.Duration) error {
	var cancel func()
	ctx, cancel = context.WithTimeout(ctx, timeout)
	defer cancel()
	tx, err := pg.NewTransaction(ctx, options)
	if err != nil {
		return err
	}
	defer rollbackWithTimeout(tx, rollbackTimeout)
	resCh := make(chan error)
	go func() {
		resCh <- worker(ctx, tx)
	}()
	var res error
	select {
	case <-ctx.Done():
		logger.Info("job ctx done")
		rollbackWithTimeout(tx, rollbackTimeout)
		logger.Info("job ctx done tx rollback finished")
		<-resCh
		logger.Info("job ctx done got result")
		close(resCh)
		res = TimeoutErr
	case res = <-resCh:
		close(resCh)
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
	defer rollbackWithTimeout(tx, rollbackTimeout)
	resCh := make(chan proto.Message)
	go func() {
		resCh <- worker(ctx, tx)
	}()
	var res proto.Message
	select {
	case <-ctx.Done():
		logger.Info("job with response ctx done")
		rollbackWithTimeout(tx, rollbackTimeout)
		logger.Info("job with response ctx done tx rollback finished")
		res = <-resCh
		logger.Info("job with response ctx done got result")
		close(resCh)
		return nil, TimeoutErr
	case res = <-resCh:
		close(resCh)
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
	defer rollbackWithTimeout(tx, rollbackTimeout)
	resCh := make(chan structs.Result)
	go func() {
		resCh <- worker(ctx, tx)
	}()
	var res structs.Result
	select {
	case <-ctx.Done():
		logger.Info("job with result ctx done")
		rollbackWithTimeout(tx, rollbackTimeout)
		logger.Info("job with result ctx done tx rollback finished")
		res = <-resCh
		logger.Info("job with result ctx done got result")
		close(resCh)
		return nil, TimeoutErr
	case res = <-resCh:
		close(resCh)
		break
	}
	return res, nil
}
