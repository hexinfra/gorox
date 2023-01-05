// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

package iface

import (
	"fmt"
	. "github.com/hexinfra/gorox/cmds/goops/rocks"
	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterHandlet("v1Handlet", func(name string, stage *Stage, app *App) Handlet {
		h := new(v1Handlet)
		h.onCreate(name, stage, app)
		return h
	})
	RegisterAppInit("iface", func(app *App) error {
		return nil
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
	h.CompInit(name)
	h.stage = stage
	h.app = app

	r := NewSimpleRouter()
	h.UseRouter(h, r)
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
