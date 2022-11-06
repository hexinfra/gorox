// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

package test

import (
	. "github.com/hexinfra/gorox/hemi"
)

func (h *testHandler) GET_form_urlencoded(req Request, resp Response) {
	resp.Send(`<form action="/form?a=bb" method="post">
	<input type="text" name="title">
	<textarea name="content"></textarea>
	<input type="submit" value="submit">
	</form>`)
}
func (h *testHandler) GET_form_multipart(req Request, resp Response) {
	resp.Send(`<form action="/form?a=bb" method="post" enctype="multipart/form-data">
	<input type="text" name="title">
	<textarea name="content"></textarea>
	<input type="submit" value="submit">
	</form>`)
}
func (h *testHandler) POST_form(req Request, resp Response) {
	resp.Push(req.Q("a"))
	resp.Push(req.F("title"))
	resp.Push(req.F("content"))
}
