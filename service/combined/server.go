package combined

import (
	"context"
	"encoding/json"
	"github.com/gorilla/mux"
	"github.com/loyal-inform/sdk-go/logger"
	"github.com/loyal-inform/sdk-go/util/consts"
	"net/http"
)

type Server struct {
	s       *http.Server
	service Service
	router  *mux.Router
}

func NewServer(service Service, address string) (*Server, error) {
	router := mux.NewRouter()
	res := &Server{
		router:  router,
		service: service,
	}
	router.HandleFunc("/connect", res.handleConnect)
	router.HandleFunc("/v2/connect", res.handleConnect)
	health := router.PathPrefix("/health").Subrouter()
	health.HandleFunc("/self", res.handleSelfHealth)
	health.HandleFunc("/other", res.handleOtherHealth)
	res.s = &http.Server{Addr: address, Handler: router}
	return res, nil
}

func (s *Server) handleConnect(w http.ResponseWriter, r *http.Request) {
	s.service.HandleConnection(w, r)
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

func (s *Server) Start() error {
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

func (s *Server) Stop() error {
	if err := s.s.Shutdown(context.Background()); err != nil {
		return err
	}
	return s.service.Stop()
}
