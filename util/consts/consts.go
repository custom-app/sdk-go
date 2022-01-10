// Package consts - полезные константы
package consts

type CtxKey string // CtxKey - тип для использования его значений в качестве ключей в контексте

const (
	ApiPrefix = "/api" // Префикс для отделения части маршрутизатора, заполняемой пользователем библиотеки

	HeaderContentType   = "Content-Type"
	HeaderXForwardedFor = "X-Forwarded-For"
	JsonContentType     = "application/json"
	ProtoContentType    = "application/x-protobuf"

	AuthHeader       = "Authorization"
	TokenStart       = "Bearer "       // Префикс значения заголовка с авторизацией
	TokenStartInd    = len(TokenStart) // Индекс, с которого в заголовке авторизации должен начинаться jwt токен
	VersionDelimiter = ":"             // Разделитель составных частей версий

	PlatformCtxKey    = CtxKey("platform")     // Ключ контекста для платформы
	VersionsCtxKey    = CtxKey("versions")     // Ключ контекста для версии
	AccountCtxKey     = CtxKey("account")      // Ключ контекста для аккаунта
	TokenNumberCtxKey = CtxKey("token_number") // Ключ контекста для номера jwt-токена при работе с авторизацией с несколькими токенами на пользователя
)
