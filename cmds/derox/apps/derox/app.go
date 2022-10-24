// Copyright (c) 2020-2022 Jingcheng Zhang <derox@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

package derox

import (
	. "github.com/hexinfra/gorox/hemi"
	"time"
)

func init() {
	RegisterHandler("deroxHandler", func(name string, stage *Stage, app *App) Handler {
		h := new(deroxHandler)
		h.init(name, stage, app)
		return h
	})
	RegisterAppInit("derox", func(app *App) error {
		return nil
	})
}

// deroxHandler
type deroxHandler struct {
	// Mixins
	Handler_
	// Assocs
	stage *Stage
	app   *App
	// States
}

func (h *deroxHandler) init(name string, stage *Stage, app *App) {
	h.SetName(name)
	h.stage = stage
	h.app = app
}

func (h *deroxHandler) OnConfigure() {
}
func (h *deroxHandler) OnPrepare() {
}
func (h *deroxHandler) OnShutdown() {
}

func (h *deroxHandler) Handle(req Request, resp Response) (next bool) {
	h.testSetCookie(req, resp)
	return
}

func (h *deroxHandler) testC(req Request, resp Response) {
	resp.Push(req.C("a"))
	resp.Push(req.C("b"))
}
func (h *deroxHandler) testQ(req Request, resp Response) {
	resp.Send(req.Q("aa"))
}
func (h *deroxHandler) testUserAgent(req Request, resp Response) {
	resp.Send(req.UserAgent())
}
func (h *deroxHandler) testSetCookie(req Request, resp Response) {
	cookie := new(Cookie)
	cookie.Set("hello", "wo r,ld")
	cookie.SetMaxAge(99)
	cookie.SetSecure()
	cookie.SetExpires(time.Now())
	resp.AddCookie(cookie)
	resp.SendBytes(nil)
}
