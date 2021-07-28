package http

import "github.com/gorilla/mux"

type Service interface {
	Start() error
	Stop() error
	FillHandlers(router *mux.Router) error
	CheckSelf() map[string]bool
	CheckOther() map[string]bool
}
