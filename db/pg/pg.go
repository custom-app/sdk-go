// Package pg - пакет для изменения API работы с PostgreSQL.
//
// Пакет основан на пакете pgx и pgxpool.
//
// В обычном sql нет возможности создать запрос и выполнить его потом, в этом основная задача этого пакета.
//
// Бонусом идет потокобезопасность, централизованное логирование ошибок.
package pg

import (
	"context"
	"fmt"
	"github.com/custom-app/sdk-go/logger"
	"github.com/jackc/pgconn"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"sync"
	"time"
)

// Scannable - интерфейс определяющий источник сканирования данных. Нужен, чтобы можно было писать одну сканирующую функцию для Request и Batch
type Scannable interface {
	Query(ctx context.Context) error
	Scan(...interface{}) error
	HaveNext() bool
	Close()
}

// RequestBuilder - интерфейс, определяющий структуру, генерирующую бд-запросы.
// Нужен, чтобы генерировать запросы не думая о том, через транзакцию это происходит или нет.
// Deprecated: Не рекомендуется использовать, не удаляется ради обратной совместимости
type RequestBuilder interface {
	NewRequest(query string, params ...interface{}) *Request
}

// TxRequestBuilder - реализация интерфейса RequestBuilder через транзакцию
// Deprecated: Не рекомендуется использовать, не удаляется ради обратной совместимости
type TxRequestBuilder struct {
	tx *Transaction
}

// NewTxRequestBuilder - создание TxRequestBuilder
// Deprecated: Не рекомендуется использовать, не удаляется ради обратной совместимости
func NewTxRequestBuilder(tx *Transaction) *TxRequestBuilder {
	return &TxRequestBuilder{
		tx: tx,
	}
}

// NewRequestBuilder - создание RequestBuilder, допускающее nil транзакцию
// Deprecated: Не рекомендуется использовать, не удаляется ради обратной совместимости
func NewRequestBuilder(tx *Transaction) RequestBuilder {
	if tx != nil {
		return NewTxRequestBuilder(tx)
	} else {
		return &DefaultRequestBuilder{}
	}
}

// NewRequest - реализация NewRequest
// Deprecated: Не рекомендуется использовать, не удаляется ради обратной совместимости
func (t *TxRequestBuilder) NewRequest(query string, params ...interface{}) *Request {
	return t.tx.NewRequest(query, params...)
}

// DefaultRequestBuilder - реализация интерфейса RequestBuilder без транзакции
// Deprecated: Не рекомендуется использовать, не удаляется ради обратной совместимости
type DefaultRequestBuilder struct {
}

// NewRequest - реализация NewRequest
// Deprecated: Не рекомендуется использовать, не удаляется ради обратной совместимости
func (d *DefaultRequestBuilder) NewRequest(query string, params ...interface{}) *Request {
	return NewRequest(query, params...)
}

var db *pgxpool.Pool

func logError(prefix string, err error) {
	logger.Info("database err: ", prefix, err)
}

// Init - инициализация пула соединений
func Init(url string) (*pgxpool.Pool, error) {
	if db != nil {
		return db, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	pool, err := pgxpool.Connect(ctx, url)
	if err != nil {
		return nil, err
	}
	db = pool
	pool.Config().MaxConnLifetime = 2 * time.Minute
	return pool, nil
}

// Ping - проверка состояния базы данных
func Ping() error {
	if db == nil {
		logger.Info("ping null db")
		return fmt.Errorf("db is nil")
	}
	rows, err := db.Query(context.Background(), "select 1")
	if err != nil {
		logger.Info("ping query failed", err)
		return err
	}
	defer rows.Close()
	var res int64
	if !rows.Next() {
		logger.Info("doesn't have next")
		return fmt.Errorf("doesn't have next")
	}
	if err := rows.Scan(&res); err != nil {
		logger.Info("ping scan failed", err)
		return err
	}
	if res != 1 {
		logger.Info("ping wrong result")
		return fmt.Errorf("false wrong result")
	}
	return nil
}

// Shutdown - Остановка пула соединений
func Shutdown() {
	if db != nil {
		db.Close()
		db = nil
	}
}

// Request - обертка запроса в базу данных
type Request struct {
	errorsCount int
	query       string
	params      []interface{}
	row         pgx.Row
	rows        pgx.Rows
	tag         pgconn.CommandTag
	err         error
	empty, next bool
	t           *Transaction
	lock        *sync.Mutex
}

func NewRequest(query string, params ...interface{}) *Request {
	return newRequestWithLock(query, &sync.Mutex{}, params...)
}

func newRequestWithLock(query string, lock *sync.Mutex, params ...interface{}) *Request {
	return &Request{
		query:  query,
		params: params,
		lock:   lock,
	}
}

func (r *Request) RowsAffected() int64 {
	return r.tag.RowsAffected()
}

func (r *Request) Exec(ctx context.Context) error {
	r.lock.Lock()
	if r.t != nil {
		r.tag, r.err = r.t.tx.Exec(ctx, r.query, r.params...)
	} else {
		r.tag, r.err = db.Exec(ctx, r.query, r.params...)
	}
	r.lock.Unlock()
	if r.err != nil {
		logError("request exec", r.err)
	}
	return r.err
}

func (r *Request) Query(ctx context.Context) error {
	r.lock.Lock()
	if r.t != nil {
		r.rows, r.err = r.t.tx.Query(ctx, r.query, r.params...)
	} else {
		r.rows, r.err = db.Query(ctx, r.query, r.params...)
	}
	if r.err == nil {
		r.next = r.rows.Next()
		r.empty = !r.next
	} else {
		logError("request query", r.err)
	}
	if r.rows.Err() != nil {
		r.err = r.rows.Err()
		if r.err != nil {
			logError("request query rows", r.err)
		}
	}
	r.lock.Unlock()
	return r.err
}

func (r *Request) QueryRow(ctx context.Context) {
	r.lock.Lock()
	if r.t != nil {
		r.row = r.t.tx.QueryRow(ctx, r.query, r.params...)
	} else {
		r.row = db.QueryRow(ctx, r.query, r.params...)
	}
	r.lock.Unlock()
}

func (r *Request) Close() {
	r.lock.Lock()
	r.rows.Close()
	r.lock.Unlock()
}

func (r *Request) HaveNext() bool {
	r.lock.Lock()
	defer r.lock.Unlock()
	if r.next {
		r.next = false
		return true
	} else if r.empty {
		return false
	} else {
		return r.rows.Next()
	}
}

func (r *Request) IsEmpty() bool {
	return r.empty
}

func (r *Request) Scan(dest ...interface{}) error {
	r.lock.Lock()
	if r.row != nil {
		r.err = r.row.Scan(dest...)
	} else if r.rows != nil {
		r.err = r.rows.Scan(dest...)
	} else {
		r.err = fmt.Errorf("missing rows")
	}
	r.lock.Unlock()
	if r.err != nil {
		logError("request scan", r.err)
	}
	return r.err
}

func (r *Request) Clone() *Request {
	return &Request{
		query:  r.query,
		params: r.params,
		t:      r.t,
		lock:   r.lock,
	}
}

// Batch - обертка группы запросов в бд
type Batch struct {
	errorsCount int
	b           *pgx.Batch
	t           *Transaction
	res         pgx.BatchResults
	rows        pgx.Rows
	row         pgx.Row
	tag         pgconn.CommandTag
	empty, next bool
	err         error
	lock        *sync.Mutex
}

func NewBatch() *Batch {
	return newBatchWithLock(&sync.Mutex{})
}

func newBatchWithLock(lock *sync.Mutex) *Batch {
	return &Batch{
		b:    &pgx.Batch{},
		lock: lock,
	}
}

func (b *Batch) AddRequest(query string, params ...interface{}) {
	b.lock.Lock()
	b.b.Queue(query, params...)
	b.lock.Unlock()
}

func (b *Batch) Send(ctx context.Context) {
	b.lock.Lock()
	if b.t != nil {
		b.res = b.t.tx.SendBatch(ctx, b.b)
	} else {
		b.res = db.SendBatch(ctx, b.b)
	}
	b.lock.Unlock()
}

func (b *Batch) RowsAffected() int64 {
	b.lock.Lock()
	defer b.lock.Unlock()
	return b.tag.RowsAffected()
}

func (b *Batch) Exec(_ context.Context) error {
	b.lock.Lock()
	b.tag, b.err = b.res.Exec()
	b.lock.Unlock()
	if b.err != nil {
		logError("batch exec", b.err)
	}
	return b.err
}

func (b *Batch) Query(_ context.Context) error {
	b.lock.Lock()
	b.rows, b.err = b.res.Query()
	if b.err == nil {
		b.next = b.rows.Next()
		b.empty = !b.next
	} else {
		logError("batch query", b.err)
	}
	if b.rows.Err() != nil {
		if b.err != nil {
			logError("batch query rows", b.err)
		}
		b.err = b.rows.Err()
	}
	b.lock.Unlock()
	return b.err
}

func (b *Batch) QueryRow() {
	b.lock.Lock()
	b.row = b.res.QueryRow()
	b.lock.Unlock()
}

func (b *Batch) Close() {
	b.lock.Lock()
	b.rows.Close()
	b.lock.Unlock()
}

func (b *Batch) Release() {
	b.lock.Lock()
	if err := b.res.Close(); err != nil {
		logError("batch release", err)
	}
	b.lock.Unlock()
}

func (b *Batch) HaveNext() bool {
	b.lock.Lock()
	defer b.lock.Unlock()
	if b.next {
		b.next = false
		return true
	} else if b.empty {
		return false
	} else {
		return b.rows.Next()
	}
}

func (b *Batch) Scan(values ...interface{}) error {
	b.lock.Lock()
	if b.row != nil {
		b.err = b.row.Scan(values...)
	} else if b.rows != nil {
		b.err = b.rows.Scan(values...)
	} else {
		b.err = fmt.Errorf("missing rows")
	}
	b.lock.Unlock()
	if b.err != nil {
		logError("batch scan err", b.err)
		return b.err
	}
	return b.err
}

// Transaction - обертка над бд транзакцией
type Transaction struct {
	logPrefix   string
	tx          pgx.Tx
	isCommitted bool
	err         error
	lock        *sync.Mutex
}

func NewTransaction(ctx context.Context, opts pgx.TxOptions) (*Transaction, error) {
	tx, err := db.BeginTx(ctx, opts)
	if err != nil {
		logError("begin tx", err)
		return nil, err
	}
	return &Transaction{
		tx:   tx,
		lock: &sync.Mutex{},
	}, nil
}

func (t *Transaction) Commit(ctx context.Context) error {
	t.lock.Lock()
	defer t.lock.Unlock()
	if err := t.tx.Commit(ctx); err != nil {
		logError("commit tx", err)
		return err
	}
	t.isCommitted = true
	return nil
}

func (t *Transaction) Rollback(ctx context.Context) {
	t.lock.Lock()
	defer t.lock.Unlock()
	if !t.isCommitted {
		if err := t.tx.Rollback(ctx); err != nil {
			logError("rollback tx", err)
		}
	}
}

// NewRequest - создание запроса в транзакции
func (t *Transaction) NewRequest(query string, params ...interface{}) *Request {
	res := newRequestWithLock(query, t.lock, params...)
	res.t = t
	return res
}

// NewBatch - создание группы запросов в транзакции
func (t *Transaction) NewBatch() *Batch {
	res := NewBatch()
	res.t = t
	return res
}
