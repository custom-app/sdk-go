package ws

import "net/http"

type Service interface {
	Start() error
	Stop() error
	HandleConnection(w http.ResponseWriter, r *http.Request)
	CheckSelf() map[string]bool
	CheckOther() map[string]bool
}
