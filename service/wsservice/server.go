package wsservice

import (
	"context"
	"encoding/json"
	"github.com/gorilla/mux"
	"github.com/loyal-inform/sdk-go/logger"
	"github.com/loyal-inform/sdk-go/util/consts"
	"net/http"
)

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

// Server - обертка над http.Server с настройкой базовой маршрутизации запросов
type Server struct {
	s       *http.Server
	handler *mux.Router
	service Service
	address string
}

// NewServer - создание сервера
func NewServer(service Service, address string) (*Server, error) {
	res := &Server{
		service: service,
		address: address,
	}
	res.handler = mux.NewRouter()
	res.handler.HandleFunc("/connect", res.handleConnect)
	res.handler.HandleFunc("/v2/connect", res.handleConnect)
	health := res.handler.PathPrefix("/health").Subrouter()
	health.HandleFunc("/self", res.handleSelfHealth)
	health.HandleFunc("/other", res.handleOtherHealth)
	return res, nil
}

func (s *Server) handleConnect(w http.ResponseWriter, r *http.Request) {
	s.service.HandleConnection(w, r)
}

// Start - запуск сервера
func (s *Server) Start() error {
	s.s = &http.Server{Addr: s.address, Handler: s.handler}
	if err := s.service.Start(); err != nil {
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
