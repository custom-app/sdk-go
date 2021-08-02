package job

import (
	"errors"
	http2 "github.com/loyal-inform/sdk-go/service/http"
	"github.com/loyal-inform/sdk-go/util/consts"
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

type HttpJob struct {
	W       http.ResponseWriter
	R       *http.Request
	Handler http.Handler
}

type HttpQueueOptions struct {
	QueueSize                         int
	Timeout                           time.Duration
	OverflowCode                      int
	OverflowMsg                       proto.Message
	OverflowMsgProto, OverflowMsgJson []byte
}

type HttpQueue struct {
	ch   chan *HttpJob
	opts *HttpQueueOptions
}

func fillOpts(opts *HttpQueueOptions) error {
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

func NewQueue(opts *HttpQueueOptions) (*HttpQueue, error) {
	if opts == nil {
		return nil, OptsRequiredErr
	}
	if err := fillOpts(opts); err != nil {
		return nil, err
	}
	res := &HttpQueue{
		ch:   make(chan *HttpJob, opts.QueueSize),
		opts: opts,
	}
	return res, nil
}

func (q *HttpQueue) AddJob(job *HttpJob) {
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

func (q *HttpQueue) Handler(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q.AddJob(&HttpJob{
			W:       w,
			R:       r,
			Handler: handler,
		})
	})
}

func (q *HttpQueue) sendOverflow(job *HttpJob) {
	if job.R.Header.Get(consts.HeaderContentType) == consts.JsonContentType {
		http2.SendBytes(job.W, q.opts.OverflowCode, q.opts.OverflowMsgJson)
	} else {
		http2.SendBytes(job.W, q.opts.OverflowCode, q.opts.OverflowMsgProto)
	}
}

func (q *HttpQueue) GetQueue() chan *HttpJob {
	return q.ch
}

func (q *HttpQueue) Close() {
	close(q.ch)
}
