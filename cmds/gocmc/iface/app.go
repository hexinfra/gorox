// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

package iface

import (
	"fmt"
	. "github.com/hexinfra/gorox/cmds/gocmc/admin"
	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterHandler("v1Handler", func(name string, stage *Stage, app *App) Handler {
		h := new(v1Handler)
		h.init(name, stage, app)
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
	admin *AdminServer
	// States
}

func (h *v1Handler) init(name string, stage *Stage, app *App) {
	h.SetName(name)
	h.stage = stage
	h.app = app

	r := NewDefaultRouter()
	h.UseRouter(h, r)
}

func (h *v1Handler) OnConfigure() {
}
func (h *v1Handler) OnPrepare() {
	h.admin = h.stage.Server("cli").(*AdminServer)
}
func (h *v1Handler) OnShutdown() {
}

func (h *v1Handler) Handle(req Request, resp Response) (next bool) {
	h.Dispatch(req, resp, nil)
	return
}

func (h *v1Handler) GET_(req Request, resp Response) {
	text := fmt.Sprintf("%d\n", h.admin.NumConns())
	resp.Send(text)
}
