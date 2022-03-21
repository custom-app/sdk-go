package basicpg_test

import (
	"context"
	"fmt"
	"github.com/custom-app/sdk-go/auth/basic"
	"github.com/custom-app/sdk-go/auth/basic/basicpg"
	"github.com/custom-app/sdk-go/db/pg"
	"github.com/custom-app/sdk-go/service/workerpool/workerpoolpg"
	"github.com/custom-app/sdk-go/structs"
	"google.golang.org/protobuf/proto"
	"log"
	"time"
)

func ExampleAuthorizationMaker_Auth() {
	// создаем очередь для управления потоком запросов в бд
	authWorkers := make([]*workerpoolpg.Worker, 2)
	authQueue := workerpoolpg.NewQueue(10)
	for i := range authWorkers {
		authWorkers[i] = workerpoolpg.NewWorker(authQueue.GetQueue())
		go authWorkers[i].Run()
	}
	defer authQueue.Close()

	// инициализируем провайдера
	provider := basicpg.NewMaker([]basicpg.RoleQuery{
		{
			Query: "select id from table_name where login=$1 and password=$2",
			Role:  1,
		},
		{
			Query: "select id from table_name2 where login=$1 and password=$2",
			Role:  2,
		},
	},
		authQueue, // очередь
		nil,       // в этом примере опущу для простоты
		time.Second)
	basic.SetDefaultAuth(provider)

	// предположим в таблице table_name2 есть запись id: 1, login: test, password: 954d5a49fd70d9b8bcdb35d252267829957f7ef7fa6c74f88419bdc5e82209f4
	res, err := basic.Auth(context.Background(), "test",
		"9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08", 0, []string{"0.0.1"})
	if err != nil {
		log.Panicln(err)
	}
	fmt.Println(res.Id, res.Role, res.Platform, res.Versions)
	// Output: 1 2 0 {0.0.1}
}

func ExampleAuthorizationMaker_AuthWithInfo() {
	// создаем очередь для управления потоком запросов в бд
	authWorkers := make([]*workerpoolpg.Worker, 2)
	authQueue := workerpoolpg.NewQueue(10)
	for i := range authWorkers {
		authWorkers[i] = workerpoolpg.NewWorker(authQueue.GetQueue())
		go authWorkers[i].Run()
	}
	defer authQueue.Close()

	// инициализируем провайдера
	provider := basicpg.NewMaker([]basicpg.RoleQuery{
		{
			Query: "select id from table_name where login=$1 and password=$2",
			Role:  1,
		},
		{
			Query: "select id from table_name2 where login=$1 and password=$2",
			Role:  2,
		},
	},
		authQueue, // очередь
		func(ctx context.Context, tx *pg.Transaction, acc *structs.Account) proto.Message {
			// код, вытягивающий данные аккаунта в Response
			return nil
		}, // в этом примере опущу для простоты
		time.Second)
	basic.SetDefaultAuth(provider)

	// предположим в таблице table_name2 есть запись id: 1, login: test, password: 954d5a49fd70d9b8bcdb35d252267829957f7ef7fa6c74f88419bdc5e82209f4
	res, resp, err := basic.AuthWithInfo(context.Background(), "test",
		"9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08", 0, []string{"0.0.1"})
	if err != nil {
		log.Panicln(err)
	}
	fmt.Println(res.Id, res.Role, res.Platform, res.Versions)
	// Output: 1 2 0 {0.0.1}
	fmt.Println(resp)
	// Output: ...
}
