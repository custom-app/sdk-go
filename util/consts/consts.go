package consts

const (
	ApiPrefix = "/api"

	HeaderContentType = "Content-Type"
	JsonContentType   = "application/json"
	ProtoContentType  = "application/x-protobuf"

	AuthHeader       = "Authorization"
	TokenStart       = "Bearer "
	TokenStartInd    = len(TokenStart)
	VersionDelimiter = ":"

	PlatformCtxKey    = "platform"
	VersionsCtxKey    = "versions"
	AccountCtxKey     = "account"
	TokenNumberCtxKey = "token_number"
)
