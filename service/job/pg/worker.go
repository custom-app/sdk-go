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

const (
	rollbackTimeout = 2 * time.Second
	commitTimeout   = 2 * time.Second
)

type DatabaseWorker func(ctx context.Context, tx *pg.Transaction) (needCommit bool, effects []func(), err error)

type DatabaseWorkerWithResponse func(ctx context.Context, tx *pg.Transaction) (
	needCommit bool, effects []func(), msg proto.Message)

type DatabaseWorkerWithResult func(ctx context.Context, tx *pg.Transaction) (needCommit bool, res structs.Result)

type errRes struct {
	needCommit bool
	effects    []func()
	err        error
}

type protoRes struct {
	needCommit bool
	effects    []func()
	msg        proto.Message
}

type structRes struct {
	needCommit bool
	res        structs.Result
}

func rollbackWithTimeout(tx *pg.Transaction, timeout time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	tx.Rollback(ctx)
}

func commitWithTimeout(tx *pg.Transaction, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return tx.Commit(ctx)
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
	resCh := make(chan errRes)
	go func() {
		var res errRes
		res.needCommit, res.effects, res.err = worker(ctx, tx)
		resCh <- res
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
	case jobRes := <-resCh:
		close(resCh)
		if jobRes.needCommit {
			if err := commitWithTimeout(tx, commitTimeout); err != nil {
				res = err
				break
			}
			for _, ef := range jobRes.effects {
				ef()
			}
		}
		res = jobRes.err
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
	resCh := make(chan protoRes)
	go func() {
		var res protoRes
		res.needCommit, res.effects, res.msg = worker(ctx, tx)
		resCh <- res
	}()
	var res proto.Message
	select {
	case <-ctx.Done():
		logger.Info("job with response ctx done")
		rollbackWithTimeout(tx, rollbackTimeout)
		logger.Info("job with response ctx done tx rollback finished")
		<-resCh
		logger.Info("job with response ctx done got result")
		close(resCh)
		return nil, TimeoutErr
	case jobRes := <-resCh:
		close(resCh)
		if jobRes.needCommit {
			if err := commitWithTimeout(tx, commitTimeout); err != nil {
				return nil, err
			}
			for _, ef := range jobRes.effects {
				ef()
			}
		}
		res = jobRes.msg
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
	resCh := make(chan structRes)
	go func() {
		var res structRes
		res.needCommit, res.res = worker(ctx, tx)
		resCh <- res
	}()
	var res structs.Result
	select {
	case <-ctx.Done():
		logger.Info("job with result ctx done")
		rollbackWithTimeout(tx, rollbackTimeout)
		logger.Info("job with result ctx done tx rollback finished")
		<-resCh
		logger.Info("job with result ctx done got result")
		close(resCh)
		return nil, TimeoutErr
	case jobRes := <-resCh:
		close(resCh)
		if jobRes.needCommit {
			if err := commitWithTimeout(tx, commitTimeout); err != nil {
				return nil, err
			}
			for _, ef := range jobRes.res.GetEffects() {
				ef()
			}
		}
		res = jobRes.res
		break
	}
	return res, nil
}
