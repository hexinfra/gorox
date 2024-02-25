// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Webdav handlets implement WebDAV protocols.

package webdav

import (
	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterHandlet("webdavHandlet", func(name string, stage *Stage, webapp *Webapp) Handlet {
		h := new(webdavHandlet)
		h.onCreate(name, stage, webapp)
		return h
	})
}

// webdavHandlet
type webdavHandlet struct {
	// Parent
	Handlet_
	// Assocs
	stage  *Stage // current stage
	webapp *Webapp
	// States
}

func (h *webdavHandlet) onCreate(name string, stage *Stage, webapp *Webapp) {
	h.MakeComp(name)
	h.stage = stage
	h.webapp = webapp
}
func (h *webdavHandlet) OnShutdown() {
	h.webapp.DecSub()
}

func (h *webdavHandlet) OnConfigure() {
	// TODO
}
func (h *webdavHandlet) OnPrepare() {
	// TODO
}

func (h *webdavHandlet) Handle(req Request, resp Response) (handled bool) {
	resp.SendBytes([]byte("not implemented yet"))
	return true
}
