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

// SendBytes - отправка массива байт со статус-кодом
func SendBytes(w http.ResponseWriter, code int, data []byte) {
	w.WriteHeader(code)
	if _, err := w.Write(data); err != nil {
		logger.Log("http send bytes err: ", err)
	}
}

// SendProto - отправка прото со статус-кодом
func SendProto(w http.ResponseWriter, code int, m proto.Message) {
	msg, err := proto.Marshal(m)
	if err != nil {
		logger.Log("http marshal proto err: ", err)
		return
	}
	w.Header().Set(consts.HeaderContentType, consts.ProtoContentType)
	SendBytes(w, code, msg)
}

// SendJson - отправка прото с помощью protojson со статус-кодом
func SendJson(w http.ResponseWriter, code int, resp proto.Message) {
	res, err := marshaler.Marshal(resp)
	if err != nil {
		logger.Log("http marshal proto to json err: ", err)
		return
	}
	w.Header().Set(consts.HeaderContentType, consts.JsonContentType)
	SendBytes(w, code, res)
}

// SendResponseWithContentType - отправка ответа на запрос с проверкой Content-Type для выбора способа сериализации ответа
func SendResponseWithContentType(w http.ResponseWriter, r *http.Request, code int, resp proto.Message) {
	if r.Header.Get(consts.HeaderContentType) == consts.JsonContentType {
		SendJson(w, code, resp)
	} else {
		SendProto(w, code, resp)
	}
}
