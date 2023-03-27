// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Rewrite handlets rewrite request path.

package rewrite

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterHandlet("rewriteHandlet", func(name string, stage *Stage, app *App) Handlet {
		h := new(rewriteHandlet)
		h.onCreate(name, stage, app)
		return h
	})
}

// rewriteHandlet
type rewriteHandlet struct {
	// Mixins
	Handlet_
	// Assocs
	stage *Stage
	app   *App
	// States
}

func (h *rewriteHandlet) onCreate(name string, stage *Stage, app *App) {
	h.MakeComp(name)
	h.stage = stage
	h.app = app
}
func (h *rewriteHandlet) OnShutdown() {
	h.app.SubDone()
}

func (h *rewriteHandlet) OnConfigure() {
}
func (h *rewriteHandlet) OnPrepare() {
}

func (h *rewriteHandlet) Handle(req Request, resp Response) (next bool) {
	// TODO
	return true
}
