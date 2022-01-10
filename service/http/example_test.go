package http_test

import (
	"github.com/gorilla/mux"
	"github.com/loyal-inform/sdk-go/service/http"
	http2 "github.com/loyal-inform/sdk-go/service/job/http"
	"log"
	"time"
)

type Service struct {
	queue   *http2.Queue
	workers []*http2.Worker
}

func (s *Service) Start() error {
	var err error
	s.queue, err = http2.NewQueue(&http2.QueueOptions{
		QueueSize:        10,
		Timeout:          time.Second,
		OverflowCode:     200,
		OverflowMsg:      nil, // сообщение о переполнении
		OverflowMsgProto: nil, // заполнится само
		OverflowMsgJson:  nil, // заполнится само
	})
	if err != nil {
		return err
	}
	s.workers = make([]*http2.Worker, 4)
	for i := range s.workers {
		s.workers[i] = http2.NewWorker(s.queue.GetQueue())
		go s.workers[i].Run()
	}
	return nil
}

func (s *Service) Stop() error {
	s.queue.Close()
	return nil
}

func (s *Service) FillHandlers(router *mux.Router) error {
	router.Handle("/test", nil) // запрос будет привязан к пути /api/test

	router.Use(s.queue.Handler) // привязка очереди запросов
	return nil
}

func (s *Service) CheckSelf() map[string]bool {
	return map[string]bool{}
}

func (s *Service) CheckOther() map[string]bool {
	return map[string]bool{}
}

func Example() {
	s, err := http.NewServer(&Service{}, "0.0.0.0:1337")
	if err != nil {
		log.Panicln(err)
	}
	go func() {
		if err := s.Start(); err != nil {
			log.Panicln(err)
		}
	}()
	time.Sleep(30 * time.Second)
	if err := s.Stop(); err != nil {
		log.Panicln(err)
	}
}
