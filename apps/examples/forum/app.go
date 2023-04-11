// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// A simple forum app.

package forum

import (
	. "github.com/hexinfra/gorox/hemi"
	"github.com/hexinfra/gorox/hemi/contrib/routers/simple"
)

func init() {
	RegisterAppInit("forum", func(app *App) error {
		return nil
	})
}

func init() {
	RegisterHandlet("forumHandlet", func(name string, stage *Stage, app *App) Handlet {
		h := new(forumHandlet)
		h.onCreate(name, stage, app)
		return h
	})
}

type forumHandlet struct {
	// Mixins
	Handlet_
	// Assocs
	stage *Stage
	app   *App
	// States
}

func (h *forumHandlet) onCreate(name string, stage *Stage, app *App) {
	h.MakeComp(name)
	h.stage = stage
	h.app = app

	r := simple.New()

	r.GET("/", h.index)

	h.SetRouter(h, r)
}
func (h *forumHandlet) OnShutdown() { h.app.SubDone() }

func (h *forumHandlet) OnConfigure() {
}
func (h *forumHandlet) OnPrepare() {
}

func (h *forumHandlet) Handle(req Request, resp Response) (next bool) {
	h.Dispatch(req, resp, h.notFound)
	return
}
func (h *forumHandlet) notFound(req Request, resp Response) {
	resp.Send("oops, target not found!")
}

func (h *forumHandlet) index(req Request, resp Response) {
	resp.Send("forum index")
}
