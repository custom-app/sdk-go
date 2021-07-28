package combined

import (
	"github.com/loyal-inform/sdk-go/service/http"
	"github.com/loyal-inform/sdk-go/service/ws"
)

type Service interface {
	http.Service
	ws.Service
}
