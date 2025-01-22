// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

package shared

import (
	. "github.com/hexinfra/gorox/hemi"
)

func (h *sharedHandlet) GET_(req ServerRequest, resp ServerResponse) { // GET empty through absolute-form, or GET /
	if req.IsAbsoluteForm() {
		resp.Send("absolute-form GET /")
	} else {
		resp.Send("origin-form GET /")
	}
}
func (h *sharedHandlet) OPTIONS_(req ServerRequest, resp ServerResponse) { // OPTIONS * or OPTIONS /
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
func (h *sharedHandlet) GET_json(req ServerRequest, resp ServerResponse) { // GET /json
	user := struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}{"jack", 25}
	resp.SendJSON(user)
}
func (h *sharedHandlet) PUT_file(req ServerRequest, resp ServerResponse) { // PUT /file
	content := req.Content()
	resp.Send(content)
}
