package httputil

import (
	"fmt"
	"net/http"

	"github.com/emicklei/go-restful"
)

type Resp struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Content interface{} `json:"content,omitempty"`
}

func NewResp(code int, message string) Resp {
	return Resp{Code: code, Message: message}
}

func Ok(resp *restful.Response) {
	resp.WriteError(http.StatusOK, restful.NewError(http.StatusOK, "")) // nolint: errcheck
}

func InternalError(resp *restful.Response, err error) {
	resp.WriteHeader(http.StatusInternalServerError)
	resp.WriteEntity(NewResp(http.StatusInternalServerError, fmt.Sprintf("server error: %v", err))) // nolint: errcheck
}

func BadRequest(resp *restful.Response, err error) {
	resp.WriteHeader(http.StatusBadRequest)
	resp.WriteEntity(NewResp(http.StatusBadRequest, fmt.Sprintf("bad request: %v", err))) // nolint: errcheck
}

func ItemNotFound(resp *restful.Response, err error) {
	resp.WriteHeader(http.StatusNotFound)
	resp.WriteEntity(NewResp(http.StatusNotFound, fmt.Sprintf("not found: %v", err))) // nolint: errcheck
}
