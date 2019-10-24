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
	resp.WriteHeaderAndEntity(http.StatusOK, NewResp(http.StatusOK, "")) // nolint: errcheck
}

func InternalError(resp *restful.Response, err error) {
	resp.WriteHeaderAndEntity(http.StatusInternalServerError, NewResp(http.StatusInternalServerError,
		fmt.Sprintf("server error: %v", err))) // nolint: errcheck
}

func BadRequest(resp *restful.Response, err error) {
	resp.WriteHeaderAndEntity(http.StatusBadRequest, NewResp(http.StatusBadRequest,
		fmt.Sprintf("bad request: %v", err))) // nolint: errcheck
}

func ItemNotFound(resp *restful.Response, err error) {
	resp.WriteHeaderAndEntity(http.StatusNotFound, NewResp(http.StatusNotFound,
		fmt.Sprintf("not found: %v", err))) // nolint: errcheck
}
