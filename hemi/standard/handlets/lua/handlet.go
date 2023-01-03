// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Lua handlets directs requests to Lua interpreter which generates responses.

package lua

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterHandlet("luaHandlet", func(name string, stage *Stage, app *App) Handlet {
		h := new(luaHandlet)
		h.onCreate(name, stage, app)
		return h
	})
}

// luaHandlet
type luaHandlet struct {
	// Mixins
	Handlet_
	// Assocs
	stage *Stage
	app   *App
	// States
}

func (h *luaHandlet) onCreate(name string, stage *Stage, app *App) {
	h.CompInit(name)
	h.stage = stage
	h.app = app
}

func (h *luaHandlet) OnConfigure() {
}
func (h *luaHandlet) OnPrepare() {
}

func (h *luaHandlet) OnShutdown() {
	h.app.SubDone()
}

func (h *luaHandlet) Handle(req Request, resp Response) (next bool) {
	resp.Send("lua")
	return
}
