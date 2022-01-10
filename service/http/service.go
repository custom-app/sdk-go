// Package http - пакет для инициализации типового http-сервера
package http

import "github.com/gorilla/mux"

// Service - интерфейс, определяющий обработчик http-запросов
type Service interface {
	Start() error                          // Запуск сервиса
	Stop() error                           // Остановка сервиса
	FillHandlers(router *mux.Router) error // Привязка обработчиков к путям
	CheckSelf() map[string]bool            // Healthcheck для сервиса
	CheckOther() map[string]bool           // Healthcheck интегрированных сервисов
}
