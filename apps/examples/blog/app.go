// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// A simple blog app.

package blog

import (
	. "github.com/hexinfra/gorox/hemi"
	"github.com/hexinfra/gorox/hemi/contrib/routers/simple"
)

func init() {
	RegisterAppInit("blog", func(app *App) error {
		return nil
	})
}

func init() {
	// Register additional handlets for your app.
	RegisterHandlet("blogHandlet", func(name string, stage *Stage, app *App) Handlet {
		h := new(blogHandlet)
		h.onCreate(name, stage, app)
		return h
	})
}

// blogHandlet
type blogHandlet struct {
	// Mixins
	Handlet_
	// Assocs
	stage *Stage // current stage
	app   *App   // associated app
	// States
}

func (h *blogHandlet) onCreate(name string, stage *Stage, app *App) {
	h.SetUp(name)
	h.stage = stage
	h.app = app

	r := simple.New()

	r.GET("/", h.index)
	r.Link("/foo", h.foo)

	h.SetRouter(h, r)
}
func (h *blogHandlet) OnShutdown() {
	h.app.SubDone()
}

func (h *blogHandlet) OnConfigure() {
}
func (h *blogHandlet) OnPrepare() {
}

func (h *blogHandlet) Handle(req Request, resp Response) (next bool) {
	h.Dispatch(req, resp, h.notFound)
	return
}
func (h *blogHandlet) notFound(req Request, resp Response) {
	resp.Send("oops, target not found!")
}

func (h *blogHandlet) index(req Request, resp Response) {
	resp.Send("blog index")
}
func (h *blogHandlet) foo(req Request, resp Response) {
	resp.Echo(req.UserAgent())
	resp.Echo(req.T("x"))
	resp.AddTrailer("y", "123")
}
