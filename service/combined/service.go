// Package combined - пакет для комбинированного http и WebSocket сервера
package combined

import (
	"github.com/loyal-inform/sdk-go/service/http"
	"github.com/loyal-inform/sdk-go/service/ws"
)

// Service - интерфейс, объединяющий интерфейсы http.Service и ws.Service
type Service interface {
	http.Service
	ws.Service
}
