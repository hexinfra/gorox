// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

package test

import (
	. "github.com/hexinfra/gorox/hemi"
	"time"
)

func init() {
	RegisterHandler("testHandler", func(name string, stage *Stage, app *App) Handler {
		h := new(testHandler)
		h.init(name, stage, app)
		return h
	})
	RegisterAppInit("test", func(app *App) error {
		return nil
	})
}

// testHandler
type testHandler struct {
	// Mixins
	Handler_
	// Assocs
	stage *Stage
	app   *App
	// States
}

func (h *testHandler) init(name string, stage *Stage, app *App) {
	h.SetName(name)
	h.stage = stage
	h.app = app

	m := NewDefaultMapper()
	h.UseMapper(h, m)
}

func (h *testHandler) OnConfigure() {
}
func (h *testHandler) OnPrepare() {
}
func (h *testHandler) OnShutdown() {
}

func (h *testHandler) Handle(req Request, resp Response) (next bool) {
	h.Dispatch(req, resp, h.notFound)
	return
}

func (h *testHandler) GET_(req Request, resp Response) {
	if req.IsAbsoluteForm() {
		resp.Send("absolute-form GET /")
	} else {
		resp.Send("origin-form GET /")
	}
}
func (h *testHandler) OPTIONS_(req Request, resp Response) {
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

func (h *testHandler) GET_setcookie(req Request, resp Response) {
	cookie1 := new(Cookie)
	cookie1.Set("hello", "wo r,ld")
	cookie1.SetMaxAge(99)
	cookie1.SetSecure()
	cookie1.SetExpires(time.Now())
	resp.AddCookie(cookie1)

	cookie2 := new(Cookie)
	cookie2.Set("world", "hello")
	resp.AddCookie(cookie2)

	resp.SendBytes(nil)
}
func (h *testHandler) GET_cookies(req Request, resp Response) {
	resp.Push(req.C("hello"))
}
func (h *testHandler) GET_querystring(req Request, resp Response) {
	resp.Send(req.QueryString())
}

func (h *testHandler) notFound(req Request, resp Response) {
	resp.Send("handle not found!")
}
