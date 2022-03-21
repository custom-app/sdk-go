package httpservice

import (
	"context"
	"encoding/json"
	"github.com/custom-app/sdk-go/logger"
	"github.com/custom-app/sdk-go/util/consts"
	"github.com/gorilla/mux"
	"net/http"
)

// Server - обертка надо http.Server с настройкой маршрутизации запросов
//
// Для привязки обработчиков будет использован не корневой роутер, а роутер с префиксом consts.ApiPrefix
type Server struct {
	s       *http.Server
	service Service
	router  *mux.Router
	address string
}

// NewServer - создание сервера
func NewServer(service Service, address string) (*Server, error) {
	router := mux.NewRouter()
	res := &Server{
		router:  router,
		service: service,
		address: address,
	}
	health := router.PathPrefix("/health").Subrouter()
	health.HandleFunc("/self", res.handleSelfHealth)
	health.HandleFunc("/other", res.handleOtherHealth)
	return res, nil
}

func (s *Server) handleSelfHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set(consts.HeaderContentType, consts.JsonContentType)
	res := s.service.CheckSelf()
	data, err := json.Marshal(res)
	if err != nil {
		w.WriteHeader(500)
		if _, err := w.Write([]byte{}); err != nil {
			logger.Info("write failed", err)
		}
		return
	}
	w.WriteHeader(200)
	if _, err := w.Write(data); err != nil {
		logger.Info("write failed", err)
	}
}

func (s *Server) handleOtherHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set(consts.HeaderContentType, consts.JsonContentType)
	res := s.service.CheckOther()
	data, err := json.Marshal(res)
	if err != nil {
		w.WriteHeader(500)
		if _, err := w.Write([]byte{}); err != nil {
			logger.Info("write failed", err)
		}
		return
	}
	w.WriteHeader(200)
	if _, err := w.Write(data); err != nil {
		logger.Info("write failed", err)
	}
}

// Start - запуск сервера
func (s *Server) Start() error {
	s.s = &http.Server{Addr: s.address, Handler: s.router}
	if err := s.service.Start(); err != nil {
		return err
	}
	if err := s.service.FillHandlers(s.router.PathPrefix(consts.ApiPrefix).Subrouter()); err != nil {
		return err
	}
	if err := s.s.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Stop - остановка сервера
func (s *Server) Stop() error {
	if err := s.s.Shutdown(context.Background()); err != nil {
		return err
	}
	return s.service.Stop()
}
