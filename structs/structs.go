package structs

import (
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

type Account struct {
	Id       int64
	Role     Role
	Platform Platform
	Versions []string
}

type SubData interface {
	GetKind() SubKind
	GetData() proto.Message
	GetAll() map[Role]bool
	GetForce() bool
	GetReceivers() map[Role][]int64
}

type Result interface {
	GetResponse() proto.Message
	GetSubs() []SubData
	GetAccountsToDrop() []*Account
}

type DefaultResult struct {
	Response proto.Message
	Subs []SubData
	AccountsToDrop []*Account
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
