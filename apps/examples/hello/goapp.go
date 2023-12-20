// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// This is a hello webapp showing how to use Gorox Webapp server to host a webapp.

package hello

import (
	"github.com/hexinfra/gorox/hemi/contrib/routers/simple"

	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterHandlet("helloHandlet", func(name string, stage *Stage, webapp *Webapp) Handlet {
		h := new(helloHandlet)
		h.onCreate(name, stage, webapp)
		return h
	})
}

// helloHandlet
type helloHandlet struct {
	// Mixins
	Handlet_
	// Assocs
	stage  *Stage  // current stage
	webapp *Webapp // associated webapp
	// States
	example string // an example config entry
}

func (h *helloHandlet) onCreate(name string, stage *Stage, webapp *Webapp) {
	h.MakeComp(name)
	h.stage = stage
	h.webapp = webapp
}
func (h *helloHandlet) OnShutdown() {
	h.webapp.SubDone()
}

func (h *helloHandlet) OnConfigure() {
	// example
	h.ConfigureString("example", &h.example, nil, "this is default value for example config entry.")
}
func (h *helloHandlet) OnPrepare() {
	r := simple.New() // you can write your own router as long as it implements the hemi.Router interface

	r.GET("/", h.index)
	r.Map("/foo", h.handleFoo)

	h.UseRouter(h, r) // equip handlet with router so it can call handles automatically through Dispatch()
}

func (h *helloHandlet) Handle(req Request, resp Response) (handled bool) {
	h.Dispatch(req, resp, h.notFound)
	return true
}
func (h *helloHandlet) notFound(req Request, resp Response) {
	resp.Send("oops, target not found!")
}

func (h *helloHandlet) index(req Request, resp Response) {
	resp.Send(h.example)
}
func (h *helloHandlet) handleFoo(req Request, resp Response) {
	resp.Echo(req.UserAgent())
	resp.Echo(req.T("x"))
	resp.AddTrailer("y", "123")
}

func (h *helloHandlet) GET_abc(req Request, resp Response) { // GET /abc
	resp.Send("this is GET /abc")
}
func (h *helloHandlet) GET_def(req Request, resp Response) { // GET /def
	resp.Send("this is GET /def")
}
func (h *helloHandlet) POST_def(req Request, resp Response) { // POST /def
	resp.Send("this is POST /def")
}
func (h *helloHandlet) GET_cookie(req Request, resp Response) { // GET /cookie
	cookie := new(Cookie)
	cookie.Set("name1", "value1")
	resp.SetCookie(cookie)
	resp.Send("this is GET /cookie")
}
