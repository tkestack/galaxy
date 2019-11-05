/*
 * Tencent is pleased to support the open source community by making TKEStack available.
 *
 * Copyright (C) 2012-2019 Tencent. All Rights Reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License"); you may not use
 * this file except in compliance with the License. You may obtain a copy of the
 * License at
 *
 * https://opensource.org/licenses/Apache-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
 * WARRANTIES OF ANY KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations under the License.
 */
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
