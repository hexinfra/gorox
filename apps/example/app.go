// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// This is an example app showing how to use Gorox application server to host an app.

package example

import (
	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	// Register additional handlers for your app.
	RegisterHandler("exampleHandler", func(name string, stage *Stage, app *App) Handler {
		h := new(exampleHandler)
		h.init(name, stage, app)
		return h
	})
	// Register initializer for your app.
	RegisterAppInit("example", func(app *App) error {
		app.AddSetting("name1", "value1") // add example setting
		return nil
	})
}

// exampleHandler
type exampleHandler struct {
	// Mixins
	Handler_
	// Assocs
	stage *Stage // current stage
	app   *App   // associated app
	// States
	example string // an example config entry
}

func (h *exampleHandler) init(name string, stage *Stage, app *App) {
	h.SetName(name)
	h.stage = stage
	h.app = app

	r := NewDefaultRouter() // you can write your own router type as long as it implements Router interface

	r.GET("/", h.handleIndex)
	r.POST("/foo", h.handleFoo)

	h.UseRouter(h, r) // equip handler with router so it can call handles automatically through Dispatch()
}

func (h *exampleHandler) OnConfigure() {
	// example
	h.ConfigureString("example", &h.example, nil, "this is default value for example config entry.")
}
func (h *exampleHandler) OnPrepare() {
	// Prepare this handler if needed
}
func (h *exampleHandler) OnShutdown() {
	// Do something if needed when this handler is shutdown
}

func (h *exampleHandler) Handle(req Request, resp Response) (next bool) {
	h.Dispatch(req, resp, nil)
	return // request is handled, next = false
}

func (h *exampleHandler) handleIndex(req Request, resp Response) {
	resp.Send(h.example)
}
func (h *exampleHandler) handleFoo(req Request, resp Response) {
	resp.Push(req.Content())
	resp.Push(req.T("x"))
	resp.AddTrailer("y", "123")
}

func (h *exampleHandler) GET_abc(req Request, resp Response) {
	resp.Send("this is GET /abc")
}
func (h *exampleHandler) POST_def(req Request, resp Response) {
	resp.Send("this is POST /def")
}
