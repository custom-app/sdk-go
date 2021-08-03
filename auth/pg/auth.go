package pg

import (
	"context"
	"github.com/loyal-inform/sdk-go/db/pg"
	"github.com/loyal-inform/sdk-go/structs"
	"google.golang.org/protobuf/proto"
)

type AccountLoader func(ctx context.Context, tx *pg.Transaction, acc *structs.Account) proto.Message
