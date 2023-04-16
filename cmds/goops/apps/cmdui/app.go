// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

package cmdui

import (
	"fmt"
	. "github.com/hexinfra/gorox/cmds/goops/srvs/rocks"
	. "github.com/hexinfra/gorox/hemi"
	"github.com/hexinfra/gorox/hemi/contrib/routers/simple"
)

func init() {
	RegisterAppInit("cmdui", func(app *App) error {
		return nil
	})
}

func init() {
	RegisterHandlet("cmdv1Handlet", func(name string, stage *Stage, app *App) Handlet {
		h := new(cmdv1Handlet)
		h.onCreate(name, stage, app)
		return h
	})
}

// cmdv1Handlet
type cmdv1Handlet struct {
	// Mixins
	Handlet_
	// Assocs
	stage *Stage
	app   *App
	rocks *RocksServer
	// States
}

func (h *cmdv1Handlet) onCreate(name string, stage *Stage, app *App) {
	h.MakeComp(name)
	h.stage = stage
	h.app = app

	r := simple.New()
	h.SetRouter(h, r)
}
func (h *cmdv1Handlet) OnShutdown() {
	h.app.SubDone()
}

func (h *cmdv1Handlet) OnConfigure() {
}
func (h *cmdv1Handlet) OnPrepare() {
	h.rocks = h.stage.Server("rocks").(*RocksServer)
}

func (h *cmdv1Handlet) Handle(req Request, resp Response) (next bool) {
	h.Dispatch(req, resp, nil)
	return
}

func (h *cmdv1Handlet) GET_(req Request, resp Response) {
	text := fmt.Sprintf("%d\n", h.rocks.NumConns())
	resp.Send(text)
}
