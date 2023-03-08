// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

package testee

import (
	. "github.com/hexinfra/gorox/hemi"
	"github.com/hexinfra/gorox/hemi/contrib/routers/simple"
)

func init() {
	RegisterAppInit("testee", func(app *App) error {
		return nil
	})
}

func init() {
	RegisterHandlet("testHandlet", func(name string, stage *Stage, app *App) Handlet {
		h := new(testHandlet)
		h.onCreate(name, stage, app)
		return h
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
	r := simple.New()
	h.SetRouter(h, r)
}
func (h *testHandlet) OnShutdown() {
	h.app.SubDone()
}

func (h *testHandlet) OnConfigure() {}
func (h *testHandlet) OnPrepare()   {}

func (h *testHandlet) Handle(req Request, resp Response) (next bool) {
	h.Dispatch(req, resp, h.notFound)
	return
}
func (h *testHandlet) notFound(req Request, resp Response) {
	resp.Send("handle not found!")
}
