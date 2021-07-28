package pg

import (
	"github.com/loyal-inform/sdk-go/db/pg"
	"github.com/loyal-inform/sdk-go/structs"
	"google.golang.org/protobuf/proto"
)

type AccountLoader func(tx *pg.Transaction, acc *structs.Account) (proto.Message, error)
