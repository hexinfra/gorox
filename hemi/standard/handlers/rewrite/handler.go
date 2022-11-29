// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Rewrite handlers rewrite request path.

package rewrite

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterHandler("rewriteHandler", func(name string, stage *Stage, app *App) Handler {
		h := new(rewriteHandler)
		h.onCreate(name, stage, app)
		return h
	})
}

// rewriteHandler
type rewriteHandler struct {
	// Mixins
	Handler_
	// Assocs
	stage *Stage
	app   *App
	// States
}

func (h *rewriteHandler) onCreate(name string, stage *Stage, app *App) {
	h.CompInit(name)
	h.stage = stage
	h.app = app
}

func (h *rewriteHandler) OnConfigure() {
}
func (h *rewriteHandler) OnPrepare() {
}

func (h *rewriteHandler) OnShutdown() {
	h.app.SubDone()
}

func (h *rewriteHandler) Handle(req Request, resp Response) (next bool) {
	// TODO
	return true
}
