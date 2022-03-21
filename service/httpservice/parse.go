package httpservice

import (
	"fmt"
	"github.com/custom-app/sdk-go/util/consts"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"io/ioutil"
	"net/http"
)

// ParseRequest - вспомогательная функция парсинга запроса
func ParseRequest(r *http.Request, res proto.Message) error {
	defer r.Body.Close()
	if r.Header.Get(consts.HeaderContentType) == consts.JsonContentType {
		return ParseJsonRequest(r, res)
	} else if r.Header.Get(consts.HeaderContentType) == consts.ProtoContentType {
		return ParseProtoRequest(r, res)
	} else {
		return fmt.Errorf("header unmatch")
	}
}

// ParseProtoRequest - функция парсинга запроса с помощью proto
func ParseProtoRequest(r *http.Request, res proto.Message) error {
	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	defer r.Body.Close()
	if err := proto.Unmarshal(data, res); err != nil {
		return err
	}
	return nil
}

// ParseJsonRequest - функция парсинга запроса с помощью protojson
func ParseJsonRequest(r *http.Request, res proto.Message) error {
	defer r.Body.Close()
	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		return err
	}
	if err := protojson.Unmarshal(data, res); err != nil {
		return err
	}
	return nil
}
