// Package workerpoolhttp - реализация очереди http-запросов с обработкой переполнения
package workerpoolhttp

import (
	"errors"
	"github.com/custom-app/sdk-go/service/httpservice"
	"github.com/custom-app/sdk-go/util/consts"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"net/http"
	"time"
)

var (
	marshaler = &protojson.MarshalOptions{
		UseProtoNames:   true,
		UseEnumNumbers:  true,
		EmitUnpopulated: true,
	}
	OptsRequiredErr = errors.New("opts required")
)

const (
	defaultBufTimeout = 10 * time.Second
	defaultQueueSize  = 4
)

// Job - таск по обработке http запроса
type Job struct {
	W       http.ResponseWriter
	R       *http.Request
	Handler http.Handler
	resCh   chan bool
}

// QueueOptions - опции очереди
type QueueOptions struct {
	QueueSize                         int           // Размер очереди
	Timeout                           time.Duration // Таймаут добавления в очередь
	OverflowCode                      int           // Статус код ошибки переполнения очереди
	OverflowMsg                       proto.Message // Сообщение о переполнении
	OverflowMsgProto, OverflowMsgJson []byte        // Сериализованное сообщение о переполнении
}

// Queue - очередь http-запросов
type Queue struct {
	ch   chan *Job
	opts *QueueOptions
}

func fillOpts(opts *QueueOptions) error {
	var err error
	if opts.OverflowMsgJson == nil {
		opts.OverflowMsgJson, err = marshaler.Marshal(opts.OverflowMsg)
		if err != nil {
			return err
		}
	}
	if opts.OverflowMsgProto == nil {
		opts.OverflowMsgProto, err = proto.Marshal(opts.OverflowMsg)
		if err != nil {
			return err
		}
	}
	if opts.Timeout == 0 {
		opts.Timeout = defaultBufTimeout
	}
	if opts.QueueSize == 0 {
		opts.QueueSize = defaultQueueSize
	}
	if opts.OverflowCode == 0 {
		opts.OverflowCode = http.StatusTooManyRequests
	}
	return nil
}

func NewQueue(opts *QueueOptions) (*Queue, error) {
	if opts == nil {
		return nil, OptsRequiredErr
	}
	if err := fillOpts(opts); err != nil {
		return nil, err
	}
	res := &Queue{
		ch:   make(chan *Job, opts.QueueSize),
		opts: opts,
	}
	return res, nil
}

func (q *Queue) AddJob(job *Job) {
	select {
	case q.ch <- job:
		break
	case <-time.After(q.opts.Timeout):
		q.sendOverflow(job)
		break
	case <-job.R.Context().Done():
		q.sendOverflow(job)
		break
	}
}

// Handler - получение функции для использования в качестве Middleware при настройке маршрутизации запросов
func (q *Queue) Handler(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resCh := make(chan bool)
		q.AddJob(&Job{
			W:       w,
			R:       r,
			Handler: handler,
			resCh:   resCh,
		})
		<-resCh
	})
}

func (q *Queue) sendOverflow(job *Job) {
	if job.R.Header.Get(consts.HeaderContentType) == consts.JsonContentType {
		httpservice.SendBytes(job.W, q.opts.OverflowCode, q.opts.OverflowMsgJson)
	} else {
		httpservice.SendBytes(job.W, q.opts.OverflowCode, q.opts.OverflowMsgProto)
	}
	job.resCh <- true
}

func (q *Queue) GetQueue() chan *Job {
	return q.ch
}

func (q *Queue) Close() {
	close(q.ch)
}
