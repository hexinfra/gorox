// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

package hemiapp

import (
	. "github.com/hexinfra/gorox/hemi"
)

func (h *hemiappHandlet) GET_(req ServerRequest, resp ServerResponse) { // GET /
	resp.Send("GET /")
}
func (h *hemiappHandlet) POST_(req ServerRequest, resp ServerResponse) { // POST /
	resp.Send("POST /")
}
func (h *hemiappHandlet) OPTIONS_(req ServerRequest, resp ServerResponse) { // OPTIONS * or OPTIONS /
	if req.IsAsteriskOptions() {
		resp.Send("OPTIONS *")
	} else {
		resp.Send("OPTIONS /")
	}
}
func (h *hemiappHandlet) GET_json(req ServerRequest, resp ServerResponse) { // GET /json
	user := struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}{"jack", 25}
	resp.SendJSON(user)
}
func (h *hemiappHandlet) PUT_file(req ServerRequest, resp ServerResponse) { // PUT /file
	content := req.Content()
	resp.Send(content)
}
