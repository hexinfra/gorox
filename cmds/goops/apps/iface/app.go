// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

package iface

import (
	"fmt"
	. "github.com/hexinfra/gorox/cmds/goops/srvs/rocks"
	. "github.com/hexinfra/gorox/hemi"
	"github.com/hexinfra/gorox/hemi/contrib/routers/simple"
)

func init() {
	RegisterAppInit("iface", func(app *App) error {
		return nil
	})
}

func init() {
	RegisterHandlet("v1Handlet", func(name string, stage *Stage, app *App) Handlet {
		h := new(v1Handlet)
		h.onCreate(name, stage, app)
		return h
	})
}

// v1Handlet
type v1Handlet struct {
	// Mixins
	Handlet_
	// Assocs
	stage *Stage
	app   *App
	rocks *RocksServer
	// States
}

func (h *v1Handlet) onCreate(name string, stage *Stage, app *App) {
	h.MakeComp(name)
	h.stage = stage
	h.app = app

	r := simple.New()
	h.SetRouter(h, r)
}
func (h *v1Handlet) OnShutdown() {
	h.app.SubDone()
}

func (h *v1Handlet) OnConfigure() {
}
func (h *v1Handlet) OnPrepare() {
	h.rocks = h.stage.Server("cli").(*RocksServer)
}

func (h *v1Handlet) Handle(req Request, resp Response) (next bool) {
	h.Dispatch(req, resp, nil)
	return
}

func (h *v1Handlet) GET_(req Request, resp Response) {
	text := fmt.Sprintf("%d\n", h.rocks.NumConns())
	resp.Send(text)
}
