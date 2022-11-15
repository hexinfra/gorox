// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Limit handlers limit clients' visiting frequency.

package limit

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterHandler("limitHandler", func(name string, stage *Stage, app *App) Handler {
		h := new(limitHandler)
		h.init(name, stage, app)
		return h
	})
}

// limitHandler
type limitHandler struct {
	// Mixins
	Handler_
	// Assocs
	stage *Stage
	app   *App
	// States
}

func (h *limitHandler) init(name string, stage *Stage, app *App) {
	h.SetName(name)
	h.stage = stage
	h.app = app
}

func (h *limitHandler) OnConfigure() {
}
func (h *limitHandler) OnPrepare() {
}
func (h *limitHandler) OnShutdown() {
	h.app.SubDone()
}

func (h *limitHandler) Handle(req Request, resp Response) (next bool) {
	// TODO
	return true
}
