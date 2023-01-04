// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

package tests

import (
	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterHandlet("testHandlet", func(name string, stage *Stage, app *App) Handlet {
		h := new(testHandlet)
		h.onCreate(name, stage, app)
		return h
	})
	RegisterAppInit("tests", func(app *App) error {
		return nil
	})
}

// testHandlet
type testHandlet struct {
	// Mixins
	Handlet_
	// Assocs
	stage *Stage
	app   *App
	// States
}

func (h *testHandlet) onCreate(name string, stage *Stage, app *App) {
	h.CompInit(name)
	h.stage = stage
	h.app = app

	r := NewSimpleRouter()
	h.UseRouter(h, r)
}
func (h *testHandlet) OnShutdown() {
	h.app.SubDone()
}

func (h *testHandlet) OnConfigure() {
}
func (h *testHandlet) OnPrepare() {
}

func (h *testHandlet) Handle(req Request, resp Response) (next bool) {
	h.Dispatch(req, resp, h.notFound)
	return
}
func (h *testHandlet) notFound(req Request, resp Response) {
	resp.Send("handle not found!")
}
