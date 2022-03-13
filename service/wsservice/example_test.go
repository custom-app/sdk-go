package wsservice_test

import (
	"github.com/loyal-inform/sdk-go/auth"
	"github.com/loyal-inform/sdk-go/auth/jwt"
	"github.com/loyal-inform/sdk-go/logger"
	"github.com/loyal-inform/sdk-go/service/workerpool/workerpoolws"
	"github.com/loyal-inform/sdk-go/service/wsservice"
	"github.com/loyal-inform/sdk-go/service/wsservice/conn"
	"github.com/loyal-inform/sdk-go/service/wsservice/opts"
	"github.com/loyal-inform/sdk-go/service/wsservice/pool"
	"github.com/loyal-inform/sdk-go/structs"
	"github.com/loyal-inform/sdk-go/util/consts"
	"google.golang.org/protobuf/proto"
	"log"
	"net/http"
	"time"
)

type Service struct {
	pool        *pool.PrivatePool
	poolWorkers []*workerpoolws.PrivateWorker
	closed      bool
}

func (s *Service) Start() error {
	var err error
	s.pool, err = pool.NewPrivatePool(&opts.ServerPrivateConnOptions{
		ServerPublicConnOptions: &opts.ServerPublicConnOptions{
			Options: &opts.Options{
				ContentType:       consts.ProtoContentType,
				OverflowMsg:       nil, // здесь nil, по факту должен быть Response
				OverflowMsgJson:   nil, // заполнится само
				OverflowMsgProto:  nil, // заполнится само
				ReceiveBufSize:    10,
				SendBufSize:       5,
				ReceiveBufTimeout: time.Second,
				PingPeriod:        45 * time.Second,
			},
		},
		AuthOptions: &opts.AuthOptions{
			BasicAllowed:   false,
			TokenAllowed:   true,
			RequestAllowed: true,
			VersionHeader:  "Test-Version",
			VersionChecker: func(platform structs.Platform, versions []string) (int, proto.Message) {
				return 200, nil
			},
			Disabled: nil,
			ErrorMapper: func(err error) (int, proto.Message) {
				if err == auth.FailedAuthErr {
					return 200, nil
				} else if err == jwt.ExpiredTokenErr {
					return 200, nil
				} else {
					return 200, nil
				}
			},
			Timeout: time.Second,
		},
	}, []structs.Role{}, 5*time.Second, 5)
	if err != nil {
		return err
	}
	s.poolWorkers = make([]*workerpoolws.PrivateWorker, 4)
	for i := range s.poolWorkers {
		s.poolWorkers[i] = workerpoolws.NewPrivateWorker(s.pool.GetQueue(), s.handleMessage)
		go s.poolWorkers[i].Run()
	}
	return nil
}

func (s *Service) handleMessage(msg *conn.PrivateMessage) {
	// parse запроса
	if acc := msg.Conn.GetAccount(); acc == nil {
		// обработка авторизации и вызов конфирма в случае успеха
		msg.Conn.AuthConfirm(&conn.AuthRes{
			Resp:    nil,
			Account: nil,
		})
	} else {
		// обработка запроса
	}
}

func (s *Service) Stop() error {
	s.closed = false
	s.pool.Close()
	return nil
}

func (s *Service) HandleConnection(w http.ResponseWriter, r *http.Request) {
	if !s.closed {
		_, err := s.pool.AddConnection(w, r)
		if err != nil {
			logger.Info("add connection failed", err)
			return
		}
	}
}

func (s *Service) CheckSelf() map[string]bool {
	return map[string]bool{}
}

func (s *Service) CheckOther() map[string]bool {
	return map[string]bool{}
}

func Example() {
	s, err := wsservice.NewServer(&Service{}, "0.0.0.0:1337")
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
