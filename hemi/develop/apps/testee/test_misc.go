// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

package testee

import (
	. "github.com/hexinfra/gorox/hemi"
)

func (h *testHandlet) GET_(req Request, resp Response) { // GET empty through absolute-form, or GET /
	if req.IsAbsoluteForm() {
		resp.Send("absolute-form GET /")
	} else {
		resp.Send("origin-form GET /")
	}
}
func (h *testHandlet) OPTIONS_(req Request, resp Response) { // OPTIONS * or OPTIONS /
	if req.IsAsteriskOptions() {
		if req.IsAbsoluteForm() {
			resp.Send("absolute-form OPTIONS *")
		} else {
			resp.Send("asterisk-form OPTIONS *")
		}
	} else {
		if req.IsAbsoluteForm() {
			resp.Send("absolute-form OPTIONS /")
		} else {
			resp.Send("origin-form OPTIONS /")
		}
	}
}
func (h *testHandlet) GET_json(req Request, resp Response) {
	user := struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}{"jack", 25}
	resp.SendJSON(user)
}
func (h *testHandlet) GET_single(req Request, resp Response) {
	accept, ok := req.Header("accept")
	if !ok {
		resp.Send("please provide accept header")
		return
	}
	resp.Send(accept)
}
func (h *testHandlet) GET_multi(req Request, resp Response) {
	accepts, ok := req.Headers("accept")
	if !ok {
		resp.Send("please provide accept header")
		return
	}
	for _, accept := range accepts {
		resp.Push(accept + "<br>")
	}
}
