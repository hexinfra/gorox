// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

package develop

import (
	. "github.com/hexinfra/gorox/hemi"
)

func (h *developHandlet) GET_(req Request, resp Response) { // GET empty through absolute-form, or GET /
	if req.IsAbsoluteForm() {
		resp.Send("absolute-form GET /")
	} else {
		resp.Send("origin-form GET /")
	}
}
func (h *developHandlet) OPTIONS_(req Request, resp Response) { // OPTIONS * or OPTIONS /
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
func (h *developHandlet) GET_json(req Request, resp Response) {
	user := struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}{"jack", 25}
	resp.SendJSON(user)
}
func (h *developHandlet) PUT_file(req Request, resp Response) {
	content := req.Content()
	resp.Send(content)
}
