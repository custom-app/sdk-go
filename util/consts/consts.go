package consts

type CtxKey string

const (
	ApiPrefix = "/api"

	HeaderContentType = "Content-Type"
	JsonContentType   = "application/json"
	ProtoContentType  = "application/x-protobuf"

	AuthHeader       = "Authorization"
	TokenStart       = "Bearer "
	TokenStartInd    = len(TokenStart)
	VersionDelimiter = ":"

	PlatformCtxKey    = CtxKey("platform")
	VersionsCtxKey    = CtxKey("versions")
	AccountCtxKey     = CtxKey("account")
	TokenNumberCtxKey = CtxKey("token_number")
)
