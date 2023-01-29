// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// This is a hello app showing how to use Gorox application server to host an app.

package hello

import (
	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	// Register additional handlets for your app.
	RegisterHandlet("helloHandlet", func(name string, stage *Stage, app *App) Handlet {
		h := new(helloHandlet)
		h.onCreate(name, stage, app)
		return h
	})
	// Register initializer for your app.
	RegisterAppInit("hello", func(app *App) error {
		app.AddSetting("name1", "value1") // add example setting
		return nil
	})
}

// helloHandlet
type helloHandlet struct {
	// Mixins
	Handlet_
	// Assocs
	stage *Stage // current stage
	app   *App   // associated app
	// States
	example string // an example config entry
}

func (h *helloHandlet) onCreate(name string, stage *Stage, app *App) {
	h.CompInit(name)
	h.stage = stage
	h.app = app

	r := NewSimpleRouter() // you can write your own router as long as it implements Router interface

	r.GET("/", h.index)
	r.Route("/foo", h.handleFoo)

	h.UseRouter(h, r) // equip handlet with router so it can call handles automatically through Dispatch()
}
func (h *helloHandlet) OnShutdown() {
	h.app.SubDone()
}

func (h *helloHandlet) OnConfigure() {
	// example
	h.ConfigureString("example", &h.example, nil, "this is default value for example config entry.")
}
func (h *helloHandlet) OnPrepare() {
}

func (h *helloHandlet) Handle(req Request, resp Response) (next bool) {
	h.Dispatch(req, resp, h.notFound)
	return // request is handled, next = false
}
func (h *helloHandlet) notFound(req Request, resp Response) {
	resp.Send("oops, not found!")
}

func (h *helloHandlet) index(req Request, resp Response) {
	resp.Send(h.example)
}
func (h *helloHandlet) handleFoo(req Request, resp Response) {
	resp.Push(req.UserAgent())
	resp.Push(req.T("x"))
	resp.AddTrailer("y", "123")
}

func (h *helloHandlet) GET_abc(req Request, resp Response) {
	resp.Send("this is GET /abc")
}
func (h *helloHandlet) POST_def(req Request, resp Response) {
	resp.Send("this is POST /def")
}
func (h *helloHandlet) GET_cookie(req Request, resp Response) {
	var cookie SetCookie
	cookie.Set("name1", "value1")
	resp.SetCookie(&cookie)
}
