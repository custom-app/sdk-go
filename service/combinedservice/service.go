// Package combinedservice - пакет для комбинированного http и WebSocket сервера
package combinedservice

import (
	"github.com/custom-app/sdk-go/service/httpservice"
	"github.com/custom-app/sdk-go/service/wsservice"
)

// Service - интерфейс, объединяющий интерфейсы http.Service и ws.Service
type Service interface {
	httpservice.Service
	wsservice.Service
}
