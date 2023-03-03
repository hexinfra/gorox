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
	RegisterHandlet("limitHandlet", func(name string, stage *Stage, app *App) Handlet {
		h := new(limitHandlet)
		h.onCreate(name, stage, app)
		return h
	})
}

// limitHandlet
type limitHandlet struct {
	// Mixins
	Handlet_
	// Assocs
	stage *Stage
	app   *App
	// States
}

func (h *limitHandlet) onCreate(name string, stage *Stage, app *App) {
	h.CompInit(name)
	h.stage = stage
	h.app = app
}
func (h *limitHandlet) OnShutdown() {
	h.app.SubDone()
}

func (h *limitHandlet) OnConfigure() {
}
func (h *limitHandlet) OnPrepare() {
}

func (h *limitHandlet) Handle(req Request, resp Response) (next bool) {
	// TODO
	return true
}
