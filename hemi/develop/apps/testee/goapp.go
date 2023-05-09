// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

package testee

import (
	"github.com/hexinfra/gorox/hemi/contrib/routers/simple"

	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterAppInit("testee", func(app *App) error {
		return nil
	})
}

func init() {
	RegisterHandlet("develHandlet", func(name string, stage *Stage, app *App) Handlet {
		h := new(develHandlet)
		h.onCreate(name, stage, app)
		return h
	})
}

// develHandlet
type develHandlet struct {
	// Mixins
	Handlet_
	// Assocs
	stage *Stage
	app   *App
	// States
}

func (h *develHandlet) onCreate(name string, stage *Stage, app *App) {
	h.MakeComp(name)
	h.stage = stage
	h.app = app
}
func (h *develHandlet) OnShutdown() {
	h.app.SubDone()
}

func (h *develHandlet) OnConfigure() {
}
func (h *develHandlet) OnPrepare() {
	r := simple.New()

	h.UseRouter(h, r)
}

func (h *develHandlet) Handle(req Request, resp Response) (next bool) {
	h.Dispatch(req, resp, h.notFound)
	return
}
func (h *develHandlet) notFound(req Request, resp Response) {
	resp.Send("handle not found!")
}
