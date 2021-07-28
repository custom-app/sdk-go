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

type Account struct {
	Id       int64
	Role     Role
	Platform Platform
	Versions []string
}

type SubData struct {
	Kind      SubKind
	Data      proto.Message
	All       map[Role]bool
	Force     bool
	Receivers map[Role][]int64
}

type Result struct {
	Response       proto.Message
	Subs           []*SubData
	AccountsToDrop []*Account
}

func NewResult(resp proto.Message, subs []*SubData, accountsToDrop []*Account) *Result {
	return &Result{
		Response:       resp,
		Subs:           subs,
		AccountsToDrop: accountsToDrop,
	}
}

type SimpleRequestProcessor func(ctx context.Context, acc *Account, req proto.Message) proto.Message

type RequestProcessor func(ctx context.Context, acc *Account, req proto.Message) *Result
