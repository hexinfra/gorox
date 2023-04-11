// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Rewriter checkers rewrite request path.

package rewriter

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterHandlet("rewriter", func(name string, stage *Stage, app *App) Handlet {
		h := new(rewriterChecker)
		h.onCreate(name, stage, app)
		return h
	})
}

// rewriterChecker
type rewriterChecker struct {
	// Mixins
	Handlet_
	// Assocs
	stage *Stage
	app   *App
	// States
}

func (h *rewriterChecker) onCreate(name string, stage *Stage, app *App) {
	h.MakeComp(name)
	h.stage = stage
	h.app = app
}
func (h *rewriterChecker) OnShutdown() {
	h.app.SubDone()
}

func (h *rewriterChecker) OnConfigure() {
}
func (h *rewriterChecker) OnPrepare() {
}

func (h *rewriterChecker) Handle(req Request, resp Response) (next bool) {
	// TODO
	return true
}
