// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

package diogin

import (
	. "github.com/hexinfra/gorox/hemi"
	"github.com/hexinfra/gorox/hemi/contrib/routers/simple"
)

func init() {
	RegisterAppInit("diogin", func(app *App) error {
		return nil
	})
}

func init() {
	RegisterHandlet("dioginHandlet", func(name string, stage *Stage, app *App) Handlet {
		h := new(dioginHandlet)
		h.onCreate(name, stage, app)
		return h
	})
}

// dioginHandlet
type dioginHandlet struct {
	// Mixins
	Handlet_
	// Assocs
	stage *Stage
	app   *App
	// States
}

func (h *dioginHandlet) onCreate(name string, stage *Stage, app *App) {
	h.CompInit(name)
	h.stage = stage
	h.app = app
	r := simple.New()
	h.SetRouter(h, r)
}
func (h *dioginHandlet) OnShutdown() {
	h.app.SubDone()
}

func (h *dioginHandlet) OnConfigure() {}
func (h *dioginHandlet) OnPrepare()   {}

func (h *dioginHandlet) Handle(req Request, resp Response) (next bool) {
	h.Dispatch(req, resp, h.notFound)
	return
}
func (h *dioginHandlet) notFound(req Request, resp Response) {
	resp.Send("handle not found!")
}
