package opts

import (
	"errors"
	http2 "github.com/loyal-inform/sdk-go/service/http"
	"github.com/loyal-inform/sdk-go/structs"
	"github.com/loyal-inform/sdk-go/util/consts"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"net/http"
	"time"
)

const (
	defaultReceiveBufSize = 10
	defaultSendBufSize    = 10
	defaultBufTimeout     = 10 * time.Second
	defaultPingPeriod     = time.Second * 45

	defaultRetryTimeout = 250 * time.Millisecond
	defaultRetryLimit   = 20
	defaultSubTimeout   = 10 * time.Second
	defaultAuthTimeout  = 10 * time.Second
)

var (
	marshaller = &protojson.MarshalOptions{
		UseProtoNames:   true,
		UseEnumNumbers:  true,
		EmitUnpopulated: true,
	}
	RequiredOptsErr = errors.New("opts required")
)

type Options struct {
	ContentType                       string
	OverflowMsg                       proto.Message
	OverflowMsgJson, OverflowMsgProto []byte
	ReceiveBufSize                    int
	SendBufSize                       int
	ReceiveBufTimeout, PingPeriod     time.Duration
}

type AuthOptions struct {
	BasicAllowed, TokenAllowed, RequestAllowed bool
	MultipleTokens                             bool
	VersionHeader                              string
	VersionChecker                             http2.VersionChecker
	Disabled                                   []structs.Role
	ErrorMapper                                http2.AuthErrorMapper
	Timeout                                    time.Duration
}

type ServerPublicConnOptions struct {
	*Options
}

type ServerPrivateConnOptions struct {
	*ServerPublicConnOptions
	AuthOptions *AuthOptions
}

type ClientPublicConnOptions struct {
	*Options
	RetryLimit              int
	RetryPeriod, SubTimeout time.Duration
	FillVersion             func(http.Header)
}

type ClientPrivateConnOptions struct {
	*ClientPublicConnOptions
	AuthTimeout time.Duration
}

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
		opts.ReceiveBufTimeout = defaultBufTimeout
	}
	if opts.PingPeriod == 0 {
		opts.PingPeriod = defaultPingPeriod
	}
	if opts.ContentType == "" {
		opts.ContentType = consts.ProtoContentType
	}
	if opts.ReceiveBufSize == 0 {
		opts.ReceiveBufSize = defaultReceiveBufSize
	}
	if opts.SendBufSize == 0 {
		opts.SendBufSize = defaultSendBufSize
	}
	return nil
}

func FillServerPublicOptions(opts *ServerPublicConnOptions) error {
	if opts.Options == nil {
		return RequiredOptsErr
	}
	return FillOpts(opts.Options)
}

func FillServerPrivateOptions(opts *ServerPrivateConnOptions) error {
	if opts.ServerPublicConnOptions == nil {
		return RequiredOptsErr
	}
	return FillServerPublicOptions(opts.ServerPublicConnOptions)
}

func FillClientPublicOptions(opts *ClientPublicConnOptions) error {
	if opts.Options == nil {
		return RequiredOptsErr
	}
	if err := FillOpts(opts.Options); err != nil {
		return err
	}
	if opts.RetryLimit == 0 {
		opts.RetryLimit = defaultRetryLimit
	}
	if opts.RetryPeriod == 0 {
		opts.RetryPeriod = defaultRetryTimeout
	}
	if opts.SubTimeout == 0 {
		opts.SubTimeout = defaultSubTimeout
	}
	return nil
}

func FillClientPrivateConnOptions(opts *ClientPrivateConnOptions) error {
	if opts.ClientPublicConnOptions == nil {
		return RequiredOptsErr
	}
	if err := FillClientPublicOptions(opts.ClientPublicConnOptions); err != nil {
		return err
	}
	if opts.AuthTimeout == 0 {
		opts.AuthTimeout = defaultAuthTimeout
	}
	return nil
}
