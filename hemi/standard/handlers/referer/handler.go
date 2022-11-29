// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Referer handlers check referer header.

package referer

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterHandler("refererHandler", func(name string, stage *Stage, app *App) Handler {
		h := new(refererHandler)
		h.onCreate(name, stage, app)
		return h
	})
}

// refererHandler
type refererHandler struct {
	// Mixins
	Handler_
	// Assocs
	stage *Stage
	app   *App
	// States
}

func (h *refererHandler) onCreate(name string, stage *Stage, app *App) {
	h.CompInit(name)
	h.stage = stage
	h.app = app
}

func (h *refererHandler) OnConfigure() {
}
func (h *refererHandler) OnPrepare() {
}

func (h *refererHandler) OnShutdown() {
	h.app.SubDone()
}

func (h *refererHandler) Handle(req Request, resp Response) (next bool) {
	// TODO
	return true
}
