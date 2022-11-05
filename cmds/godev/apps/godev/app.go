// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

package godev

import (
	. "github.com/hexinfra/gorox/hemi"
	"time"
)

func init() {
	RegisterHandler("godevHandler", func(name string, stage *Stage, app *App) Handler {
		h := new(godevHandler)
		h.init(name, stage, app)
		return h
	})
	RegisterAppInit("godev", func(app *App) error {
		return nil
	})
}

// godevHandler
type godevHandler struct {
	// Mixins
	Handler_
	// Assocs
	stage *Stage
	app   *App
	// States
}

func (h *godevHandler) init(name string, stage *Stage, app *App) {
	h.Handler_.Init(name, h)
	h.stage = stage
	h.app = app
}

func (h *godevHandler) OnConfigure() {
}
func (h *godevHandler) OnPrepare() {
}
func (h *godevHandler) OnShutdown() {
	// Do nothing.
}

func (h *godevHandler) Handle(req Request, resp Response) (next bool) {
	h.Dispatch(req, resp)
	return
}

func (h *godevHandler) GET_a(req Request, resp Response) {
	resp.Push(req.C("a"))
	resp.Push(req.C("b"))
}
func (h *godevHandler) GET_b(req Request, resp Response) {
	resp.Send(req.Q("aa"))
}
func (h *godevHandler) GET_c(req Request, resp Response) {
	resp.Send(req.UserAgent())
}
func (h *godevHandler) GET_d(req Request, resp Response) {
	cookie := new(Cookie)
	cookie.Set("hello", "wo r,ld")
	cookie.SetMaxAge(99)
	cookie.SetSecure()
	cookie.SetExpires(time.Now())
	resp.AddCookie(cookie)
	resp.SendBytes(nil)
}
func (h *godevHandler) GET_e(req Request, resp Response) {
	resp.Send(req.QueryString())
}
