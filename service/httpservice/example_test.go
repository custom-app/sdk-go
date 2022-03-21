package httpservice_test

import (
	"github.com/custom-app/sdk-go/service/httpservice"
	"github.com/custom-app/sdk-go/service/workerpool/workerpoolhttp"
	"github.com/gorilla/mux"
	"log"
	"time"
)

type Service struct {
	queue   *workerpoolhttp.Queue
	workers []*workerpoolhttp.Worker
}

func (s *Service) Start() error {
	var err error
	s.queue, err = workerpoolhttp.NewQueue(&workerpoolhttp.QueueOptions{
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
	s.workers = make([]*workerpoolhttp.Worker, 4)
	for i := range s.workers {
		s.workers[i] = workerpoolhttp.NewWorker(s.queue.GetQueue())
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
	s, err := httpservice.NewServer(&Service{}, "0.0.0.0:1337")
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
