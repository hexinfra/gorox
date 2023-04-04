// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Limit handlets limit clients' visiting frequency.

package limit

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterHandlet("limitChecker", func(name string, stage *Stage, app *App) Handlet {
		h := new(limitChecker)
		h.onCreate(name, stage, app)
		return h
	})
}

// limitChecker
type limitChecker struct {
	// Mixins
	Handlet_
	// Assocs
	stage *Stage
	app   *App
	// States
}

func (h *limitChecker) onCreate(name string, stage *Stage, app *App) {
	h.MakeComp(name)
	h.stage = stage
	h.app = app
}
func (h *limitChecker) OnShutdown() {
	h.app.SubDone()
}

func (h *limitChecker) OnConfigure() {
}
func (h *limitChecker) OnPrepare() {
}

func (h *limitChecker) Handle(req Request, resp Response) (next bool) {
	// TODO
	return true
}
