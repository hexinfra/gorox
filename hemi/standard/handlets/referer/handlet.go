// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Referer handlets check referer header.

package referer

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterHandlet("refererHandlet", func(name string, stage *Stage, app *App) Handlet {
		h := new(refererHandlet)
		h.onCreate(name, stage, app)
		return h
	})
}

// refererHandlet
type refererHandlet struct {
	// Mixins
	Handlet_
	// Assocs
	stage *Stage
	app   *App
	// States
}

func (h *refererHandlet) onCreate(name string, stage *Stage, app *App) {
	h.CompInit(name)
	h.stage = stage
	h.app = app
}
func (h *refererHandlet) OnShutdown() {
	h.app.SubDone()
}

func (h *refererHandlet) OnConfigure() {
}
func (h *refererHandlet) OnPrepare() {
}

func (h *refererHandlet) Handle(req Request, resp Response) (next bool) {
	// TODO
	return true
}
