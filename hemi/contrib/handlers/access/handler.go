// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Access handlers allow limiting access to certain client addresses.

package access

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterHandler("accessHandler", func(name string, stage *Stage, app *App) Handler {
		h := new(accessHandler)
		h.init(name, stage, app)
		return h
	})
}

// accessHandler
type accessHandler struct {
	// Mixins
	Handler_
	// Assocs
	stage *Stage
	app   *App
	// States
}

func (h *accessHandler) init(name string, stage *Stage, app *App) {
	h.CompInit(name)
	h.stage = stage
	h.app = app
}

func (h *accessHandler) OnConfigure() {
}
func (h *accessHandler) OnPrepare() {
}
func (h *accessHandler) OnShutdown() {
	h.app.SubDone()
}

func (h *accessHandler) Handle(req Request, resp Response) (next bool) {
	// TODO
	return true
}
