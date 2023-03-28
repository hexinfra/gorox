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
	RegisterHandlet("testeeHandlet", func(name string, stage *Stage, app *App) Handlet {
		h := new(testeeHandlet)
		h.onCreate(name, stage, app)
		return h
	})
}

// testeeHandlet
type testeeHandlet struct {
	// Mixins
	Handlet_
	// Assocs
	stage *Stage
	app   *App
	// States
}

func (h *testeeHandlet) onCreate(name string, stage *Stage, app *App) {
	h.MakeComp(name)
	h.stage = stage
	h.app = app

	r := simple.New()

	h.SetRouter(h, r)
}
func (h *testeeHandlet) OnShutdown() {
	h.app.SubDone()
}

func (h *testeeHandlet) OnConfigure() {}
func (h *testeeHandlet) OnPrepare()   {}

func (h *testeeHandlet) Handle(req Request, resp Response) (next bool) {
	h.Dispatch(req, resp, h.notFound)
	return
}
func (h *testeeHandlet) notFound(req Request, resp Response) {
	resp.Send("handle not found!")
}
