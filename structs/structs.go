// Package structs содержит в себе объявления типов, использующихся везде в этом SDK
package structs

import (
	"google.golang.org/protobuf/proto"
)

type Role int32     // enum для роли
type SubKind int32  // enum для топика подписки
type Platform int32 // enum для платформы

// Account - структура содержащая иммутабельную информацию о клиенте
type Account struct {
	Id       int64    // id клиента
	Role     Role     // роль клиента
	Platform Platform // платформа, с которой выполнен запрос
	Versions []string // версия системы, с которой выполнен запрос
}

// SubData - интерфейс, определяющий сообщения для подписки
type SubData interface {
	GetKind() SubKind               // топик подписки
	GetData() proto.Message         // произвольные данные
	GetAll() map[Role]bool          // роли пользователей, которым надо послать сообщение без какой-либо фильтрации
	GetForce() bool                 // Deprecated: использовалось для обратной совместимости
	GetReceivers() map[Role][]int64 // списки id пользователей, которым надо послать сообщение
}

// Result - интерфейс, определяющий результат работы произвольного метода
type Result interface {
	GetResponse() proto.Message    // ответ на запрос
	GetSubs() []SubData            // список сообщений по подписке
	GetAccountsToDrop() []*Account // список аккаунтов, с которыми надо разорвать соединение
	GetEffects() []func()          // список эффектов, которые должны быть применены в случае успешной обработки результата
}

// DefaultResult - вспомогательная структура, реализующая интерфейс Result
type DefaultResult struct {
	Response       proto.Message
	Subs           []SubData
	AccountsToDrop []*Account
	Effects        []func()
}

func (d *DefaultResult) GetResponse() proto.Message {
	return d.Response
}

func (d *DefaultResult) GetSubs() []SubData {
	return d.Subs
}

func (d *DefaultResult) GetAccountsToDrop() []*Account {
	return d.AccountsToDrop
}

func (d *DefaultResult) GetEffects() []func() {
	return d.Effects
}
