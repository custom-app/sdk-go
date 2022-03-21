// Package opts - пакет с опциями Websocket соединений
package opts

import (
	"errors"
	"github.com/custom-app/sdk-go/service/httpservice"
	"github.com/custom-app/sdk-go/structs"
	"github.com/custom-app/sdk-go/util/consts"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"net/http"
	"time"
)

const (
	DefaultReceiveBufSize = 10               // Размер входящего буфера по умолчанию
	DefaultSendBufSize    = 10               // Размер исходящего буфера по умолчанию
	DefaultBufTimeout     = 10 * time.Second // Таймаут входящего буфера по умолчанию
	DefaultPingPeriod     = time.Second * 45 // Период пинга по умолчанию

	DefaultRetryPeriod = 250 * time.Millisecond // Период попыток переподключения по умолчанию
	DefaultRetryLimit  = 20                     // Лимит попыток переподключения
	DefaultSubTimeout  = 10 * time.Second       // Таймаут получения ответа на запрос подписки
	DefaultAuthTimeout = 10 * time.Second       // Таймаут получения ответа на запрос авторизации
)

var (
	marshaller = &protojson.MarshalOptions{
		UseProtoNames:   true,
		UseEnumNumbers:  true,
		EmitUnpopulated: true,
	}
	RequiredOptsErr = errors.New("opts required")
)

// Options - базовые опции websocket-соединения
type Options struct {
	ContentType                       string        // Content Type для соединения. В этом формате будут передаваться сообщения
	OverflowMsg                       proto.Message // Сообщение о переполнении входящего буфера
	OverflowMsgJson, OverflowMsgProto []byte        // Сериализованные сообщения о переполнении
	ReceiveBufSize                    int           // Размер входящего буфера
	SendBufSize                       int           // Размер исходящего буфера
	ReceiveBufTimeout                 time.Duration // Таймаут входящего буфера
	PingPeriod                        time.Duration // Период пинга
}

// AuthOptions - опции авторизации соединения
type AuthOptions struct {
	// Допускается ли авторизация с помощью basic-авторизации, jwt токена и запроса, посланного после инициализации сокета
	BasicAllowed, TokenAllowed, RequestAllowed bool
	VersionHeader                              string                      // Имя заголовка с версией
	VersionChecker                             httpservice.VersionChecker  // Функция проверки версии
	Disabled                                   []structs.Role              // Список ролей, которым запрещено подключение
	ErrorMapper                                httpservice.AuthErrorMapper // Преобразователь ошибок авторизации в формат API системы
	Timeout                                    time.Duration               // Таймаут авторизации по запросу
}

// ServerPublicConnOptions - опции для публичных соединений. Используется для сервера
type ServerPublicConnOptions struct {
	*Options
}

// ServerPrivateConnOptions - опции для авторизованных соединений. Используется для сервера
type ServerPrivateConnOptions struct {
	*ServerPublicConnOptions
	AuthOptions *AuthOptions
}

// ClientPublicConnOptions - опции для публичных соединений. Используется для клиентских соединений
type ClientPublicConnOptions struct {
	*Options
	RetryLimit              int               // Лимит количества попыток переподключения
	RetryPeriod, SubTimeout time.Duration     // Период попыток переподключения и таймаут ответа на запрос подписки
	FillVersion             func(http.Header) // Функция заполнения заголовка с версией
	NeedRestart             bool              // Нужно ли переподключаться в случае разрыва соединения
}

// ClientPrivateConnOptions - опции для авторизованных соединений. Используется для клиентских соединений
type ClientPrivateConnOptions struct {
	*ClientPublicConnOptions
	AuthTimeout time.Duration // Таймаут ответа на запрос авторизации
}

// FillOpts - вспомогательная функция заполнения опция значениями по умолчанию
func FillOpts(opts *Options) error {
	var err error
	if opts.OverflowMsgJson == nil {
		opts.OverflowMsgJson, err = marshaller.Marshal(opts.OverflowMsg)
		if err != nil {
			return err
		}
	}
	if opts.OverflowMsgProto == nil {
		opts.OverflowMsgProto, err = proto.Marshal(opts.OverflowMsg)
		if err != nil {
			return err
		}
	}
	if opts.ReceiveBufTimeout == 0 {
		opts.ReceiveBufTimeout = DefaultBufTimeout
	}
	if opts.PingPeriod == 0 {
		opts.PingPeriod = DefaultPingPeriod
	}
	if opts.ContentType == "" {
		opts.ContentType = consts.ProtoContentType
	}
	if opts.ReceiveBufSize == 0 {
		opts.ReceiveBufSize = DefaultReceiveBufSize
	}
	if opts.SendBufSize == 0 {
		opts.SendBufSize = DefaultSendBufSize
	}
	return nil
}

// FillServerPublicOptions - вспомогательная функция заполнения опция значениями по умолчанию
func FillServerPublicOptions(opts *ServerPublicConnOptions) error {
	if opts.Options == nil {
		return RequiredOptsErr
	}
	return FillOpts(opts.Options)
}

// FillServerPrivateOptions - вспомогательная функция заполнения опция значениями по умолчанию
func FillServerPrivateOptions(opts *ServerPrivateConnOptions) error {
	if opts.ServerPublicConnOptions == nil {
		return RequiredOptsErr
	}
	return FillServerPublicOptions(opts.ServerPublicConnOptions)
}

// FillClientPublicOptions - вспомогательная функция заполнения опция значениями по умолчанию
func FillClientPublicOptions(opts *ClientPublicConnOptions) error {
	if opts.Options == nil {
		return RequiredOptsErr
	}
	if err := FillOpts(opts.Options); err != nil {
		return err
	}
	if opts.RetryLimit == 0 {
		opts.RetryLimit = DefaultRetryLimit
	}
	if opts.RetryPeriod == 0 {
		opts.RetryPeriod = DefaultRetryPeriod
	}
	if opts.SubTimeout == 0 {
		opts.SubTimeout = DefaultSubTimeout
	}
	return nil
}

// FillClientPrivateConnOptions - вспомогательная функция заполнения опция значениями по умолчанию
func FillClientPrivateConnOptions(opts *ClientPrivateConnOptions) error {
	if opts.ClientPublicConnOptions == nil {
		return RequiredOptsErr
	}
	if err := FillClientPublicOptions(opts.ClientPublicConnOptions); err != nil {
		return err
	}
	if opts.AuthTimeout == 0 {
		opts.AuthTimeout = DefaultAuthTimeout
	}
	return nil
}
