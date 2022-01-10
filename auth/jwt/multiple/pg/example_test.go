package pg_test

import (
	"context"
	"fmt"
	"github.com/loyal-inform/sdk-go/auth/jwt"
	"github.com/loyal-inform/sdk-go/auth/jwt/multiple"
	pg3 "github.com/loyal-inform/sdk-go/auth/jwt/multiple/pg"
	"github.com/loyal-inform/sdk-go/db/pg"
	pg2 "github.com/loyal-inform/sdk-go/service/job/pg"
	"github.com/loyal-inform/sdk-go/structs"
	"google.golang.org/protobuf/proto"
	"log"
	"time"
)

func ExampleAuthorizationMaker_Auth() {
	// создаем очередь для управления потоком запросов в бд
	authWorkers := make([]*pg2.Worker, 2)
	authQueue := pg2.NewQueue(10)
	for i := range authWorkers {
		authWorkers[i] = pg2.NewWorker(authQueue.GetQueue())
		go authWorkers[i].Run()
	}
	defer authQueue.Close()

	// инициализируем провайдера
	provider := pg3.NewMaker(map[structs.Role]string{
		0: "first_role_tokens",
		1: "second_role_tokens",
	}, "r3vbrb3b3fb3", authQueue, nil, time.Minute, time.Hour, time.Second)
	multiple.SetDefaultAuth(provider)

	// допустим, секрет токена найдется в таблице first_role_tokens
	res, num, err := multiple.Auth(context.Background(), "jwt-token", jwt.PurposeAccess, 0, []string{"0.0.1"})
	if err != nil {
		log.Panicln(err)
	}
	fmt.Println(res.Id, res.Role, res.Platform, res.Versions, num)
	// Output: 1 0 0 {0.0.1} 0
}

func ExampleAuthorizationMaker_AuthWithInfo() {
	// создаем очередь для управления потоком запросов в бд
	authWorkers := make([]*pg2.Worker, 2)
	authQueue := pg2.NewQueue(10)
	for i := range authWorkers {
		authWorkers[i] = pg2.NewWorker(authQueue.GetQueue())
		go authWorkers[i].Run()
	}
	defer authQueue.Close()

	// инициализируем провайдера
	provider := pg3.NewMaker(map[structs.Role]string{
		0: "first_role_tokens",
		1: "second_role_tokens",
	}, "r3vbrb3b3fb3", authQueue,
		func(ctx context.Context, tx *pg.Transaction, acc *structs.Account) proto.Message {
			// код, вытягивающий данные аккаунта в Response
			return nil
		}, time.Minute, time.Hour, time.Second)
	multiple.SetDefaultAuth(provider)

	// допустим, секрет токена найдется в таблице first_role_tokens
	res, num, resp, err := multiple.AuthWithInfo(context.Background(), "jwt-token", jwt.PurposeAccess, 0, []string{"0.0.1"})
	if err != nil {
		log.Panicln(err)
	}
	fmt.Println(res.Id, res.Role, res.Platform, res.Versions, num)
	// Output: 1 0 0 {0.0.1} 0
	fmt.Println(resp)
	// Output: ...
}

func ExampleAuthorizationMaker_CreateTokens() {
	// создаем очередь для управления потоком запросов в бд
	authWorkers := make([]*pg2.Worker, 2)
	authQueue := pg2.NewQueue(10)
	for i := range authWorkers {
		authWorkers[i] = pg2.NewWorker(authQueue.GetQueue())
		go authWorkers[i].Run()
	}
	defer authQueue.Close()

	// инициализируем провайдера
	provider := pg3.NewMaker(map[structs.Role]string{
		0: "first_role_tokens",
		1: "second_role_tokens",
	}, "r3vbrb3b3fb3", authQueue, nil, time.Minute, time.Hour, time.Second)
	multiple.SetDefaultAuth(provider)

	// создаем токен
	accessToken, accessExpiresAt, refreshToken, refreshExpiresAt, err := multiple.CreateTokens(context.Background(), 0, 1)
	if err != nil {
		log.Panicln(err)
	}
	fmt.Println(accessToken, accessExpiresAt)
	// Output: token now + time.Minute
	fmt.Println(refreshToken, refreshExpiresAt)
	// Output: token now + time.Hour
}

func ExampleAuthorizationMaker_ReCreateTokens() {
	// создаем очередь для управления потоком запросов в бд
	authWorkers := make([]*pg2.Worker, 2)
	authQueue := pg2.NewQueue(10)
	for i := range authWorkers {
		authWorkers[i] = pg2.NewWorker(authQueue.GetQueue())
		go authWorkers[i].Run()
	}
	defer authQueue.Close()

	// инициализируем провайдера
	provider := pg3.NewMaker(map[structs.Role]string{
		0: "first_role_tokens",
		1: "second_role_tokens",
	}, "r3vbrb3b3fb3", authQueue, nil, time.Minute, time.Hour, time.Second)
	multiple.SetDefaultAuth(provider)

	// пересоздаем токен
	accessToken, accessExpiresAt, refreshToken, refreshExpiresAt, err := multiple.ReCreateTokens(context.Background(), 0, 1, 0)
	if err != nil {
		log.Panicln(err)
	}
	fmt.Println(accessToken, accessExpiresAt)
	// Output: token now + time.Minute
	fmt.Println(refreshToken, refreshExpiresAt)
	// Output: token now + time.Hour
}
