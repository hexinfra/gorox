// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Webdav handlets implement WebDAV protocols.

package webdav

import (
	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterHandlet("webdavHandlet", func(compName string, stage *Stage, webapp *Webapp) Handlet {
		h := new(webdavHandlet)
		h.onCreate(compName, stage, webapp)
		return h
	})
}

// webdavHandlet
type webdavHandlet struct {
	// Parent
	Handlet_
	// States
}

func (h *webdavHandlet) onCreate(compName string, stage *Stage, webapp *Webapp) {
	h.Handlet_.OnCreate(compName, stage, webapp)
}
func (h *webdavHandlet) OnShutdown() {
	h.Webapp().DecHandlet()
}

func (h *webdavHandlet) OnConfigure() {
	// TODO
}
func (h *webdavHandlet) OnPrepare() {
	// TODO
}

func (h *webdavHandlet) Handle(req ServerRequest, resp ServerResponse) (handled bool) {
	resp.SendBytes([]byte("not implemented yet"))
	return true
}
