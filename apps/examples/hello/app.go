// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// This is a hello app showing how to use Gorox application server to host an app.

package hello

import (
	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	// Register additional handlers for your app.
	RegisterHandler("helloHandler", func(name string, stage *Stage, app *App) Handler {
		h := new(helloHandler)
		h.init(name, stage, app)
		return h
	})
	// Register initializer for your app.
	RegisterAppInit("hello", func(app *App) error {
		app.AddSetting("name1", "value1") // add example setting
		return nil
	})
}

// helloHandler
type helloHandler struct {
	// Mixins
	Handler_
	// Assocs
	stage *Stage // current stage
	app   *App   // associated app
	// States
	example string // an example config entry
}

func (h *helloHandler) init(name string, stage *Stage, app *App) {
	h.SetName(name)
	h.stage = stage
	h.app = app

	r := NewSimpleRouter() // you can write your own router as long as it implements Router interface

	r.GET("/", h.index)
	r.POST("/foo", h.handleFoo)

	h.UseRouter(h, r) // equip handler with router so it can call handles automatically through Dispatch()
}

func (h *helloHandler) OnConfigure() {
	// example
	h.ConfigureString("example", &h.example, nil, "this is default value for example config entry.")
}
func (h *helloHandler) OnPrepare()  {}
func (h *helloHandler) OnShutdown() {}

func (h *helloHandler) Handle(req Request, resp Response) (next bool) {
	h.Dispatch(req, resp, h.notFound)
	return // request is handled, next = false
}
func (h *helloHandler) notFound(req Request, resp Response) {
	resp.Send("oops, not found!")
}

func (h *helloHandler) index(req Request, resp Response) {
	resp.Send(h.example)
}
func (h *helloHandler) handleFoo(req Request, resp Response) {
	resp.Push(req.Content())
	resp.Push(req.T("x"))
	resp.AddTrailer("y", "123")
}

func (h *helloHandler) GET_abc(req Request, resp Response) {
	resp.Send("this is GET /abc")
}
func (h *helloHandler) POST_def(req Request, resp Response) {
	resp.Send("this is POST /def")
}
