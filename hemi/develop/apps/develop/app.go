// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

package develop

import (
	"github.com/hexinfra/gorox/hemi/contrib/routers/simple"

	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterAppInit("develop", func(app *App) error {
		return nil
	})
}

func init() {
	RegisterHandlet("developHandlet", func(name string, stage *Stage, app *App) Handlet {
		h := new(developHandlet)
		h.onCreate(name, stage, app)
		return h
	})
}

// developHandlet
type developHandlet struct {
	// Mixins
	Handlet_
	// Assocs
	stage *Stage
	app   *App
	// States
}

func (h *developHandlet) onCreate(name string, stage *Stage, app *App) {
	h.MakeComp(name)
	h.stage = stage
	h.app = app
}
func (h *developHandlet) OnShutdown() {
	h.app.SubDone()
}

func (h *developHandlet) OnConfigure() {}
func (h *developHandlet) OnPrepare() {
	r := simple.New()

	h.UseRouter(h, r)
}

func (h *developHandlet) Handle(req Request, resp Response) (next bool) {
	h.Dispatch(req, resp, h.notFound)
	return
}
func (h *developHandlet) notFound(req Request, resp Response) {
	resp.Send("handle not found!")
}
