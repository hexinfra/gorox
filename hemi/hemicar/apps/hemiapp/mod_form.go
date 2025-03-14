// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

package hemiapp

import (
	"fmt"

	. "github.com/hexinfra/gorox/hemi"
)

func (h *hemiappHandlet) GET_form_urlencoded(req ServerRequest, resp ServerResponse) {
	resp.Send(`<form action="/form?a=aa&b=bb" method="post">
	<input type="text" name="title">
	<textarea name="content"></textarea>
	<input type="submit" value="submit">
	</form>`)
}
func (h *hemiappHandlet) GET_form_multipart(req ServerRequest, resp ServerResponse) {
	resp.Send(`<form action="/form?a=aa&b=bb" method="post" enctype="multipart/form-data">
	<input type="text" name="title">
	<textarea name="content"></textarea>
	<input type="submit" value="submit">
	</form>`)
}
func (h *hemiappHandlet) POST_form(req ServerRequest, resp ServerResponse) {
	a := req.Q("a")
	b := req.Q("b")
	title := req.F("title")
	content := req.F("content")
	resp.Send(fmt.Sprintf("a=%s b=%s title=%s content=%s", a, b, title, content))
}
