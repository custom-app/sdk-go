package http

import (
	"github.com/loyal-inform/sdk-go/logger"
	"github.com/loyal-inform/sdk-go/util/consts"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"net/http"
)

var marshaler = &protojson.MarshalOptions{
	UseProtoNames:   true,
	UseEnumNumbers:  true,
	EmitUnpopulated: true,
}

func SendBytes(w http.ResponseWriter, code int, data []byte) {
	w.WriteHeader(code)
	if _, err := w.Write(data); err != nil {
		logger.Log("http send bytes err: ", err)
	}
}

func SendProto(w http.ResponseWriter, code int, m proto.Message) {
	msg, err := proto.Marshal(m)
	if err != nil {
		logger.Log("http marshal proto err: ", err)
		return
	}
	w.Header().Set(consts.HeaderContentType, consts.ProtoContentType)
	SendBytes(w, code, msg)
}

func SendJson(w http.ResponseWriter, code int, resp proto.Message) {
	res, err := marshaler.Marshal(resp)
	if err != nil {
		logger.Log("http marshal proto to json err: ", err)
		return
	}
	w.Header().Set(consts.HeaderContentType, consts.JsonContentType)
	SendBytes(w, code, res)
}

func SendResponseWithContentType(w http.ResponseWriter, r *http.Request, code int, resp proto.Message) {
	if r.Header.Get(consts.HeaderContentType) == consts.JsonContentType {
		SendJson(w, code, resp)
	} else if r.Header.Get(consts.HeaderContentType) == consts.ProtoContentType {
		SendProto(w, code, resp)
	}
}
