// Copyright (c) 2020-2023 Feng Wei <feng19910104@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

package fengve

import (
	"github.com/hexinfra/gorox/hemi/contrib/routers/simple"

	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterAppInit("fengve", func(app *App) error {
		return nil
	})
}

func init() {
	RegisterHandlet("fengveHandlet", func(name string, stage *Stage, app *App) Handlet {
		h := new(fengveHandlet)
		h.onCreate(name, stage, app)
		return h
	})
}

// fengveHandlet
type fengveHandlet struct {
	// Mixins
	Handlet_
	// Assocs
	stage *Stage
	app   *App
	// States
}

func (h *fengveHandlet) onCreate(name string, stage *Stage, app *App) {
	h.MakeComp(name)
	h.stage = stage
	h.app = app

	r := simple.New()
	r.GET("/abc", h.abc)

	h.SetRouter(h, r)
}
func (h *fengveHandlet) OnShutdown() {
	h.app.SubDone()
}

func (h *fengveHandlet) OnConfigure() {}
func (h *fengveHandlet) OnPrepare()   {}

func (h *fengveHandlet) Handle(req Request, resp Response) (next bool) {
	h.Dispatch(req, resp, h.notFound)
	return
}
func (h *fengveHandlet) notFound(req Request, resp Response) {
	resp.Send("handle not found!")
}
func (h *fengveHandlet) abc(req Request, resp Response) {
	resp.Send("abc")
}
