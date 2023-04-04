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
	RegisterHandlet("rewriter", func(name string, stage *Stage, app *App) Handlet {
		h := new(rewriteChecker)
		h.onCreate(name, stage, app)
		return h
	})
}

// rewriteChecker
type rewriteChecker struct {
	// Mixins
	Handlet_
	// Assocs
	stage *Stage
	app   *App
	// States
}

func (h *rewriteChecker) onCreate(name string, stage *Stage, app *App) {
	h.MakeComp(name)
	h.stage = stage
	h.app = app
}
func (h *rewriteChecker) OnShutdown() {
	h.app.SubDone()
}

func (h *rewriteChecker) OnConfigure() {
}
func (h *rewriteChecker) OnPrepare() {
}

func (h *rewriteChecker) Handle(req Request, resp Response) (next bool) {
	// TODO
	return true
}
