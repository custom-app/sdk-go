package structs

import (
	"context"
	"google.golang.org/protobuf/proto"
)

type Role int32
type SubKind int32
type Platform int32
type Purpose int32

const (
	PurposeAccess = Purpose(iota)
	PurposeRefresh
)

type Account interface {
	GetRole() Role
	GetId() int64
	GetPlatform() Platform
	GetVersions() string
}

type DefaultAccount struct {
	Id       int64
	Role     Role
	Platform Platform
	Versions []string
}

func (a *DefaultAccount) GetId() int64 {
	return a.Id
}

func (a *DefaultAccount) GetRole() Role {
	return a.Role
}

func (a *DefaultAccount) GetPlatform() Platform {
	return a.Platform
}

func (a *DefaultAccount) GetVersions() []string {
	return a.Versions
}

type SubData struct {
	Kind      SubKind
	Data      proto.Message
	Filters   map[Role]func(acc Account) bool
	Force     bool
	Receivers map[Role][]int64
}

type AccountUpdate struct {
	Acc      Account
	Id       int64
	Role     Role
	NeedDrop bool
}

type Result struct {
	Response       proto.Message
	Subs           []*SubData
	AccountUpdates []*AccountUpdate
}

func NewResult(resp proto.Message, subs []*SubData, accounts []*AccountUpdate) *Result {
	return &Result{
		Response:       resp,
		Subs:           subs,
		AccountUpdates: accounts,
	}
}

func FilterAll() func(account *Account) bool {
	return func(account *Account) bool {
		return true
	}
}

type SimpleRequestProcessor func(ctx context.Context, acc Account, req proto.Message) proto.Message

type RequestProcessor func(ctx context.Context, acc Account, req proto.Message) *Result
