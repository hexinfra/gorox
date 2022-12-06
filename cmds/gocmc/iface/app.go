// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

package iface

import (
	"fmt"
	. "github.com/hexinfra/gorox/cmds/gocmc/rocks"
	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterHandler("v1Handler", func(name string, stage *Stage, app *App) Handler {
		h := new(v1Handler)
		h.onCreate(name, stage, app)
		return h
	})
	RegisterAppInit("iface", func(app *App) error {
		return nil
	})
}

// v1Handler
type v1Handler struct {
	// Mixins
	Handler_
	// Assocs
	stage *Stage
	app   *App
	rocks *RocksServer
	// States
}

func (h *v1Handler) onCreate(name string, stage *Stage, app *App) {
	h.CompInit(name)
	h.stage = stage
	h.app = app

	r := NewSimpleRouter()
	h.UseRouter(h, r)
}

func (h *v1Handler) OnConfigure() {
}
func (h *v1Handler) OnPrepare() {
	h.rocks = h.stage.Server("cli").(*RocksServer)
}

func (h *v1Handler) OnShutdown() {
	h.app.SubDone()
}

func (h *v1Handler) Handle(req Request, resp Response) (next bool) {
	h.Dispatch(req, resp, nil)
	return
}

func (h *v1Handler) GET_(req Request, resp Response) {
	text := fmt.Sprintf("%d\n", h.rocks.NumConns())
	resp.Send(text)
}
