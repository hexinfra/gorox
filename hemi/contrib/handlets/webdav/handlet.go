// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Webdav handlets implement WebDAV protocols.

package webdav

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterHandlet("webdavHandlet", func(name string, stage *Stage, app *App) Handlet {
		h := new(webdavHandlet)
		h.onCreate(name, stage, app)
		return h
	})
}

// webdavHandlet
type webdavHandlet struct {
	// Mixins
	Handlet_
	// Assocs
	stage *Stage
	app   *App
	// States
}

func (h *webdavHandlet) onCreate(name string, stage *Stage, app *App) {
	h.CompInit(name)
	h.stage = stage
	h.app = app
}
func (h *webdavHandlet) OnShutdown() {
	h.app.SubDone()
}

func (h *webdavHandlet) OnConfigure() {
}
func (h *webdavHandlet) OnPrepare() {
}

func (h *webdavHandlet) Handle(req Request, resp Response) (next bool) {
	resp.SendBytes([]byte("not implemented yet"))
	return
}
