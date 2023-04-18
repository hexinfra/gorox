// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Access checkers allow limiting access to certain client addresses.

package access

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterHandlet("accessChecker", func(name string, stage *Stage, app *App) Handlet {
		h := new(accessChecker)
		h.onCreate(name, stage, app)
		return h
	})
}

// accessChecker
type accessChecker struct {
	// Mixins
	Handlet_
	// Assocs
	stage *Stage
	app   *App
	// States
}

func (h *accessChecker) onCreate(name string, stage *Stage, app *App) {
	h.MakeComp(name)
	h.stage = stage
	h.app = app
}
func (h *accessChecker) OnShutdown() {
	h.app.SubDone()
}

func (h *accessChecker) OnConfigure() {
	// TODO
}
func (h *accessChecker) OnPrepare() {
	// TODO
}

func (h *accessChecker) Handle(req Request, resp Response) (next bool) {
	// TODO
	return true
}
