package http

import (
	"context"
	"encoding/json"
	"github.com/gorilla/mux"
	"github.com/loyal-inform/sdk-go/util/consts"
	"log"
	"net/http"
)

type Server struct {
	s       *http.Server
	service Service
	router  *mux.Router
}

func NewServer(service Service, address string) (*Server, error) {
	router := mux.NewRouter()
	if err := service.FillHandlers(router.PathPrefix(consts.ApiPrefix).Subrouter()); err != nil {
		return nil, err
	}
	res := &Server{
		router:  router,
		service: service,
	}
	health := router.PathPrefix("/health").Subrouter()
	health.HandleFunc("/self", res.handleSelfHealth)
	health.HandleFunc("/other", res.handleOtherHealth)
	res.s = &http.Server{Addr: address, Handler: router}
	return res, nil
}

func (s *Server) handleSelfHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set(consts.HeaderContentType, consts.JsonContentType)
	res := s.service.CheckSelf()
	data, err := json.Marshal(res)
	if err != nil {
		w.WriteHeader(500)
		if _, err := w.Write([]byte{}); err != nil {
			log.Println("write failed", err)
		}
		return
	}
	w.WriteHeader(200)
	if _, err := w.Write(data); err != nil {
		log.Println("write failed", err)
	}
}

func (s *Server) handleOtherHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set(consts.HeaderContentType, consts.JsonContentType)
	res := s.service.CheckOther()
	data, err := json.Marshal(res)
	if err != nil {
		w.WriteHeader(500)
		if _, err := w.Write([]byte{}); err != nil {
			log.Println("write failed", err)
		}
		return
	}
	w.WriteHeader(200)
	if _, err := w.Write(data); err != nil {
		log.Println("write failed", err)
	}
}

func (s *Server) Start() error {
	if err := s.service.Start(); err != nil {
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
