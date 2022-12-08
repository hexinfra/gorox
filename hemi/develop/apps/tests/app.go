// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

package tests

import (
	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterHandler("testHandler", func(name string, stage *Stage, app *App) Handler {
		h := new(testHandler)
		h.onCreate(name, stage, app)
		return h
	})
	RegisterAppInit("tests", func(app *App) error {
		return nil
	})
}

// testHandler
type testHandler struct {
	// Mixins
	Handler_
	// Assocs
	stage *Stage
	app   *App
	// States
}

func (h *testHandler) onCreate(name string, stage *Stage, app *App) {
	h.CompInit(name)
	h.stage = stage
	h.app = app

	r := NewSimpleRouter()
	h.UseRouter(h, r)
}

func (h *testHandler) OnConfigure() {
}
func (h *testHandler) OnPrepare() {
}

func (h *testHandler) OnShutdown() {
	h.app.SubDone()
}

func (h *testHandler) Handle(req Request, resp Response) (next bool) {
	h.Dispatch(req, resp, h.notFound)
	return
}
func (h *testHandler) notFound(req Request, resp Response) {
	resp.Send("handle not found!")
}
