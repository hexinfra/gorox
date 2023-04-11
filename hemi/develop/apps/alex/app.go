// Copyright (c) 2020-2023 Zhang Jingcheng <alex@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

package alex

import (
	. "github.com/hexinfra/gorox/hemi"
	"github.com/hexinfra/gorox/hemi/contrib/routers/simple"
)

func init() {
	RegisterAppInit("alex", func(app *App) error {
		return nil
	})
}

func init() {
	RegisterHandlet("alexHandlet", func(name string, stage *Stage, app *App) Handlet {
		h := new(alexHandlet)
		h.onCreate(name, stage, app)
		return h
	})
}

// alexHandlet
type alexHandlet struct {
	// Mixins
	Handlet_
	// Assocs
	stage *Stage
	app   *App
	// States
}

func (h *alexHandlet) onCreate(name string, stage *Stage, app *App) {
	h.MakeComp(name)
	h.stage = stage
	h.app = app

	r := simple.New()
	r.GET("/abc", h.abc)

	h.SetRouter(h, r)
}
func (h *alexHandlet) OnShutdown() {
	h.app.SubDone()
}

func (h *alexHandlet) OnConfigure() {}
func (h *alexHandlet) OnPrepare()   {}

func (h *alexHandlet) Handle(req Request, resp Response) (next bool) {
	h.Dispatch(req, resp, h.notFound)
	return
}
func (h *alexHandlet) notFound(req Request, resp Response) {
	resp.Send("handle not found!")
}
func (h *alexHandlet) abc(req Request, resp Response) {
	resp.Send("abc")
}
