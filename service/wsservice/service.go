// Package wsservice - пакет для инициализации типового WebSocket-сервера
package wsservice

import "net/http"

// Service - интерфейс, определяющий обработчик Websocket-соединений
type Service interface {
	Start() error                                            // Запуск сервиса
	Stop() error                                             // Остановка сервиса
	HandleConnection(w http.ResponseWriter, r *http.Request) // Обработка запроса на соединение
	CheckSelf() map[string]bool                              // Healthcheck для сервиса
	CheckOther() map[string]bool                             // Healthcheck интегрированных сервисов
}
