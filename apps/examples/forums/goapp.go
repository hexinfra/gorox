// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// A simple forums app.

package forums

import (
	"github.com/hexinfra/gorox/hemi/contrib/routers/simple"

	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterAppInit("forums", func(app *App) error {
		return nil
	})
}

func init() {
	RegisterHandlet("forumsHandlet", func(name string, stage *Stage, app *App) Handlet {
		h := new(forumsHandlet)
		h.onCreate(name, stage, app)
		return h
	})
}

type forumsHandlet struct {
	// Mixins
	Handlet_
	// Assocs
	stage *Stage
	app   *App
	// States
}

func (h *forumsHandlet) onCreate(name string, stage *Stage, app *App) {
	h.MakeComp(name)
	h.stage = stage
	h.app = app
}
func (h *forumsHandlet) OnShutdown() {
	h.app.SubDone()
}

func (h *forumsHandlet) OnConfigure() {
}
func (h *forumsHandlet) OnPrepare() {
	r := simple.New()

	r.GET("/", h.index)

	h.UseRouter(h, r)
}

func (h *forumsHandlet) Handle(req Request, resp Response) (next bool) {
	h.Dispatch(req, resp, h.notFound)
	return
}
func (h *forumsHandlet) notFound(req Request, resp Response) {
	resp.Send("oops, target not found!")
}

func (h *forumsHandlet) index(req Request, resp Response) {
	resp.Send("forums index")
}
