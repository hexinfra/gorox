// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Referer checkers check referer header.

package referer

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterHandlet("refererChecker", func(name string, stage *Stage, app *App) Handlet {
		h := new(refererChecker)
		h.onCreate(name, stage, app)
		return h
	})
}

// refererChecker
type refererChecker struct {
	// Mixins
	Handlet_
	// Assocs
	stage *Stage
	app   *App
	// States
}

func (h *refererChecker) onCreate(name string, stage *Stage, app *App) {
	h.MakeComp(name)
	h.stage = stage
	h.app = app
}
func (h *refererChecker) OnShutdown() {
	h.app.SubDone()
}

func (h *refererChecker) OnConfigure() {
}
func (h *refererChecker) OnPrepare() {
}

func (h *refererChecker) Handle(req Request, resp Response) (next bool) {
	// TODO
	return true
}
