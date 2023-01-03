// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Gatex handlets implement API Gateway.

package gatex

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterHandlet("gatex", func(name string, stage *Stage, app *App) Handlet {
		h := new(gatex)
		h.onCreate(name, stage, app)
		return h
	})
}

// gatex
type gatex struct {
	// Mixins
	Handlet_
	// Assocs
	stage *Stage
	app   *App
	// States
}

func (h *gatex) onCreate(name string, stage *Stage, app *App) {
	h.CompInit(name)
	h.stage = stage
	h.app = app
}

func (h *gatex) OnConfigure() {
}
func (h *gatex) OnPrepare() {
}

func (h *gatex) OnShutdown() {
	h.app.SubDone()
}

func (h *gatex) Handle(req Request, resp Response) (next bool) {
	// TODO
	return
}
