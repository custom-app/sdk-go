package apiclient_test

import (
	"context"
	"github.com/custom-app/sdk-go/service/apiclient"
	"github.com/custom-app/sdk-go/service/wsservice/opts"
	"google.golang.org/protobuf/proto"
	"log"
	"net/http"
)

func ExampleWsClient_SendMessage() {
	client, err := apiclient.NewWsClient([]*apiclient.ConnectionInfo{
		{
			Address:     "ws://localhost:1337/connect",
			ConnOptions: &opts.ClientPrivateConnOptions{}, // подробно останавливаться не буду
			SubHandler: func(msg proto.Message) (err error) {
				// обработка сообщения по подписке
				return nil
			},
		},
	}, "access token", 236135, "refresh token", 3612623,
		func(ctx context.Context, token string) (string, int64, string, int64, error) {
			// код получения новых токенов, как правило, http запрос в сервис авторизации с refresh токенов
			return "", 0, "", 0, nil
		}, func(err error) {
			// код работы с ошибкой (логирование, отправка в тг чат багов)
		}, func(bytes []byte) (apiclient.WsMessageKind, apiclient.WsMessage, error) {
			var res proto.Message // в реальности здесь Request
			if err := proto.Unmarshal(bytes, res); err != nil {
				return 0, nil, err
			}
			// определяем тип сообщения по распаршенному прото и возвращаем
			return 0, nil, nil
		})
	if err != nil {
		log.Panicln(err)
	}
	if err := client.Start(); err != nil {
		log.Panicln(err)
	}
	if err := client.SendMessage(0, nil, func(msg proto.Message) (needRetry bool, err error) {
		// обработка ответа на запрос
		//
		// может быть необходимо получить какие-то данные из ответа
		//
		// при возврате needRetry == true запрос будет переотправлен
		return false, nil
	}); err != nil {
		log.Panicln(err)
	}
}

func ExampleHttpClient_MakeRequest() {
	client, err := apiclient.NewHttpClient("https://host:port", "access token", 23513,
		"refresh token", 263216, func(header http.Header) {
			// добавление версии в заголовки запроса
		}, func(ctx context.Context, token string) (string, int64, string, int64, error) {
			// код получения новых токенов, как правило, http запрос в сервис авторизации с refresh токенов
			return "", 0, "", 0, nil
		}, func(err error) {
			// код работы с ошибкой (логирование, отправка в тг чат багов)
		})
	if err != nil {
		log.Panicln(err)
	}
	if err := client.Start(); err != nil {
		log.Panicln(err)
	}
	client.MakeRequest(context.Background(), nil,
		func(msg []byte) (needRetry bool, err error) {
			// парсинг сообщения, получение нужной информации
			//
			// при необходимости можно повторить попытку отправки запроса
			return false, nil
		})
}
