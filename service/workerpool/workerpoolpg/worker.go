package workerpoolpg

import (
	"context"
	"github.com/custom-app/sdk-go/db/pg"
	"github.com/custom-app/sdk-go/structs"
	"github.com/jackc/pgx/v4"
	"google.golang.org/protobuf/proto"
	"time"
)

const (
	rollbackTimeout = 2 * time.Second
	commitTimeout   = 2 * time.Second
)

// DatabaseWorker - стандартное действие с транзакцией
type DatabaseWorker func(ctx context.Context, tx *pg.Transaction) (needCommit bool, res *JobResultErr)

// DatabaseWorkerWithResponse - действие с транзакцией, подразумевающее прото результат
type DatabaseWorkerWithResponse func(ctx context.Context, tx *pg.Transaction) (
	needCommit bool, res *JobResultProto)

// DatabaseWorkerWithResult - действие с транзакцией, подразумевающее результат типа structs.Result
type DatabaseWorkerWithResult func(ctx context.Context, tx *pg.Transaction) (needCommit bool, res structs.Result)

// JobResultErr - результат работы с транзакцией
type JobResultErr struct {
	needCommit bool
	Effects    []func() // Эффекты, которые надо применить в случае успеха коммита транзакции
	Err        error    // Ошибка
}

func WrapError(err error) *JobResultErr {
	return &JobResultErr{
		Err: err,
	}
}

// JobResultProto - результат работы с транзакцией
type JobResultProto struct {
	needCommit bool
	Effects    []func()      // Эффекты, которые надо применить в случае успеха коммита транзакции
	Msg        proto.Message // Прото результат
}

func WrapMessage(msg proto.Message) *JobResultProto {
	return &JobResultProto{
		Msg: msg,
	}
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

// MakeJob - выполнение работы с транзакцией с таймаутом
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
	resCh := make(chan *JobResultErr)
	go func() {
		needCommit, res := worker(ctx, tx)
		if res == nil {
			res = &JobResultErr{}
		}
		res.needCommit = needCommit
		resCh <- res
	}()
	var res error
	select {
	case <-ctx.Done():
		<-resCh
		close(resCh)
		res = TimeoutErr
	case jobRes := <-resCh:
		close(resCh)
		if jobRes.needCommit {
			if err := commitWithTimeout(tx, commitTimeout); err != nil {
				res = err
				break
			}
			for _, ef := range jobRes.Effects {
				ef()
			}
		}
		res = jobRes.Err
		break
	}
	return res
}

// MakeJobWithResponse - выполнение работы с транзакцией с прото результатом с таймаутом
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
	resCh := make(chan *JobResultProto)
	go func() {
		needCommit, res := worker(ctx, tx)
		if res == nil {
			res = &JobResultProto{}
		}
		res.needCommit = needCommit
		resCh <- res
	}()
	var res proto.Message
	select {
	case <-ctx.Done():
		<-resCh
		close(resCh)
		return nil, TimeoutErr
	case jobRes := <-resCh:
		close(resCh)
		if jobRes.needCommit {
			if err := commitWithTimeout(tx, commitTimeout); err != nil {
				return nil, err
			}
			for _, ef := range jobRes.Effects {
				ef()
			}
		}
		res = jobRes.Msg
		break
	}
	return res, nil
}

// MakeJobWithResult - выполнение работы с транзакцией с structs.Result результатом(где подписки и т. д.) с таймаутом
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
		<-resCh
		close(resCh)
		return nil, TimeoutErr
	case jobRes := <-resCh:
		close(resCh)
		if jobRes.needCommit {
			if err := commitWithTimeout(tx, commitTimeout); err != nil {
				return nil, err
			}
			if jobRes.res != nil {
				for _, ef := range jobRes.res.GetEffects() {
					ef()
				}
			}
		}
		res = jobRes.res
		break
	}
	return res, nil
}
