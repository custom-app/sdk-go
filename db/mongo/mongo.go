package mongo

import (
	"context"
	"github.com/loyal-inform/sdk-go/logger"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"sync"
	"time"
)

var db *mongo.Client

// Init - инициализация клиента mongodb
func Init(url string) (*mongo.Client, error) {
	if db != nil {
		return db, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(url))
	if err != nil {
		return nil, err
	}
	defer func() {
		if err = client.Disconnect(ctx); err != nil {
			panic(err)
		}
	}()
	db = client
	return client, nil
}

// Shutdown - остановка клиента mongodb
func Shutdown(ctx context.Context) {
	if db != nil {
		if err := db.Disconnect(ctx); err != nil {
			panic(err)
		}
		db = nil
	}
}

// Transaction - обертка над бд транзакцией mongodb
type Transaction struct {
	logPrefix   string
	session     mongo.Session
	isCommitted bool
	err         error
	lock        *sync.Mutex
}

func NewTransaction(ctx context.Context) (*Transaction, error) {
	sess, err := db.StartSession()
	if err != nil {
		return nil, err
	}
	defer sess.EndSession(ctx)
	if err := sess.StartTransaction(); err != nil {
		logError("begin tx", err)
		return nil, err
	}
	return &Transaction{
		session: sess,
		lock:    &sync.Mutex{},
	}, nil
}

func (t *Transaction) Commit(ctx context.Context) error {
	t.lock.Lock()
	defer t.lock.Unlock()
	if err := t.session.CommitTransaction(ctx); err != nil {
		logError("commit tx", err)
		return err
	}
	t.isCommitted = true
	return nil
}

func (t *Transaction) Abort(ctx context.Context) {
	t.lock.Lock()
	defer t.lock.Unlock()
	if !t.isCommitted {
		if err := t.session.AbortTransaction(ctx); err != nil {
			logError("rollback tx", err)
		}
	}
}

func logError(s string, err error) {
	logger.Info("mongo database err: ", s, err)
}
