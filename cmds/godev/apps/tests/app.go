// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

package tests

import (
	. "github.com/hexinfra/gorox/hemi"
	"time"
)

func init() {
	RegisterHandler("testsHandler", func(name string, stage *Stage, app *App) Handler {
		h := new(testsHandler)
		h.init(name, stage, app)
		return h
	})
	RegisterAppInit("tests", func(app *App) error {
		return nil
	})
}

// testsHandler
type testsHandler struct {
	// Mixins
	Handler_
	// Assocs
	stage *Stage
	app   *App
	// States
}

func (h *testsHandler) init(name string, stage *Stage, app *App) {
	h.SetName(name)
	h.stage = stage
	h.app = app

	m := NewDefaultMapper()
	h.UseMapper(h, m)
}

func (h *testsHandler) OnConfigure() {
}
func (h *testsHandler) OnPrepare() {
}
func (h *testsHandler) OnShutdown() {
}

func (h *testsHandler) Handle(req Request, resp Response) (next bool) {
	h.Dispatch(req, resp, h.notFound)
	return
}

func (h *testsHandler) GET_(req Request, resp Response) {
	if req.IsAbsoluteForm() {
		resp.Send("absolute-form GET /")
	} else {
		resp.Send("origin-form GET /")
	}
}
func (h *testsHandler) OPTIONS_(req Request, resp Response) {
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
func (h *testsHandler) GET_a(req Request, resp Response) {
	resp.Push(req.C("a"))
	resp.Push(req.C("b"))
}
func (h *testsHandler) GET_b(req Request, resp Response) {
	resp.Send(`<form action="/c?a=aa" method="post" enctype="multipart/form-data">
	<input type="text" name="title">
	<textarea name="content"></textarea>
	<input type="submit" value="submit">
	</form>`)
}
func (h *testsHandler) POST_c(req Request, resp Response) {
	resp.Push(req.Q("a"))
	resp.Push(req.F("title"))
	resp.Push(req.F("content"))
}
func (h *testsHandler) GET_d(req Request, resp Response) {
	cookie := new(Cookie)
	cookie.Set("hello", "wo r,ld")
	cookie.SetMaxAge(99)
	cookie.SetSecure()
	cookie.SetExpires(time.Now())
	resp.AddCookie(cookie)
	resp.SendBytes(nil)
}
func (h *testsHandler) GET_e(req Request, resp Response) {
	resp.Send(req.QueryString())
}

func (h *testsHandler) notFound(req Request, resp Response) {
	resp.Send("handle not found!")
}
