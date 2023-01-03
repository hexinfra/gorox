// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Access handlets allow limiting access to certain client addresses.

package access

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterHandlet("accessHandlet", func(name string, stage *Stage, app *App) Handlet {
		h := new(accessHandlet)
		h.onCreate(name, stage, app)
		return h
	})
}

// accessHandlet
type accessHandlet struct {
	// Mixins
	Handlet_
	// Assocs
	stage *Stage
	app   *App
	// States
}

func (h *accessHandlet) onCreate(name string, stage *Stage, app *App) {
	h.CompInit(name)
	h.stage = stage
	h.app = app
}

func (h *accessHandlet) OnConfigure() {
}
func (h *accessHandlet) OnPrepare() {
}

func (h *accessHandlet) OnShutdown() {
	h.app.SubDone()
}

func (h *accessHandlet) Handle(req Request, resp Response) (next bool) {
	// TODO
	return true
}
