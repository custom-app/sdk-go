package job

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v4"
	"github.com/loyal-inform/sdk-go/db/pg"
	"github.com/loyal-inform/sdk-go/structs"
	"google.golang.org/protobuf/proto"
	"time"
)

var (
	TimeoutError = errors.New("operation timeout")
)

type DatabaseQueue struct {
	ch chan bool
}

type DatabaseWorker func(ctx context.Context, tx *pg.Transaction) error

type DatabaseWorkerWithResponse func(ctx context.Context, tx *pg.Transaction) proto.Message

type DatabaseWorkerWithResult func(ctx context.Context, tx *pg.Transaction) *structs.Result

func NewQueue(size int) *DatabaseQueue {
	res := &DatabaseQueue{
		ch: make(chan bool, size),
	}
	for i := 0; i < size; i++ {
		res.ch <- true
	}
	return res
}

func (q *DatabaseQueue) acquireResources(ctx context.Context) error {
	select {
	case <-q.ch:
		return nil
	case <-ctx.Done():
		return TimeoutError
	}
}

func (q *DatabaseQueue) releaseResource() {
	q.ch <- true
}

func (q *DatabaseQueue) MakeJob(ctx context.Context, options pgx.TxOptions,
	worker DatabaseWorker, timeout time.Duration) error {
	var cancel func()
	ctx, cancel = context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := q.acquireResources(ctx); err != nil {
		return err
	}
	defer q.releaseResource()
	tx, err := pg.NewTransaction(options)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	resCh := make(chan error)
	go func() {
		resCh <- worker(ctx, tx)
	}()
	var res error
	select {
	case <-ctx.Done():
		tx.Rollback()
		res = <-resCh
		break
	case res = <-resCh:
		break
	}
	return res
}

func (q *DatabaseQueue) MakeJobWithResponse(ctx context.Context, options pgx.TxOptions,
	worker DatabaseWorkerWithResponse, timeout time.Duration) (proto.Message, error) {
	var cancel func()
	ctx, cancel = context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := q.acquireResources(ctx); err != nil {
		return nil, err
	}
	defer q.releaseResource()
	tx, err := pg.NewTransaction(options)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	resCh := make(chan proto.Message)
	go func() {
		resCh <- worker(ctx, tx)
	}()
	var res proto.Message
	select {
	case <-ctx.Done():
		tx.Rollback()
		res = <-resCh
		break
	case res = <-resCh:
		break
	}
	return res, nil
}

func (q *DatabaseQueue) MakeJobWithResult(ctx context.Context, options pgx.TxOptions,
	worker DatabaseWorkerWithResult, timeout time.Duration) (*structs.Result, error) {
	var cancel func()
	ctx, cancel = context.WithTimeout(ctx, timeout)
	defer cancel()
	if err := q.acquireResources(ctx); err != nil {
		return nil, err
	}
	defer q.releaseResource()
	tx, err := pg.NewTransaction(options)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	resCh := make(chan *structs.Result)
	go func() {
		resCh <- worker(ctx, tx)
	}()
	var res *structs.Result
	select {
	case <-ctx.Done():
		tx.Rollback()
		res = <-resCh
		break
	case res = <-resCh:
		break
	}
	return res, nil
}
