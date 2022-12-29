// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Lua handlers directs requests to Lua interpreter which generates responses.

package lua

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterHandler("luaHandler", func(name string, stage *Stage, app *App) Handler {
		h := new(luaHandler)
		h.onCreate(name, stage, app)
		return h
	})
}

// luaHandler
type luaHandler struct {
	// Mixins
	Handler_
	// Assocs
	stage *Stage
	app   *App
	// States
}

func (h *luaHandler) onCreate(name string, stage *Stage, app *App) {
	h.CompInit(name)
	h.stage = stage
	h.app = app
}

func (h *luaHandler) OnConfigure() {
}
func (h *luaHandler) OnPrepare() {
}

func (h *luaHandler) OnShutdown() {
	h.app.SubDone()
}

func (h *luaHandler) Handle(req Request, resp Response) (next bool) {
	resp.Send("lua")
	return
}
