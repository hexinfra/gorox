// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HTTPS checkers redirect request urls to its HTTPS version.

package https

import (
	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterHandlet("httpsChecker", func(name string, stage *Stage, webapp *Webapp) Handlet {
		h := new(httpsChecker)
		h.onCreate(name, stage, webapp)
		return h
	})
}

// httpsChecker
type httpsChecker struct {
	// Parent
	Handlet_
	// Assocs
	stage  *Stage // current stage
	webapp *Webapp
	// States
	permanent bool
	authority string
}

func (h *httpsChecker) onCreate(name string, stage *Stage, webapp *Webapp) {
	h.MakeComp(name)
	h.stage = stage
	h.webapp = webapp
}
func (h *httpsChecker) OnShutdown() {
	h.webapp.DecSub()
}

func (h *httpsChecker) OnConfigure() {
	// permanent
	h.ConfigureBool("permanent", &h.permanent, false)
	// authority
	h.ConfigureString("authority", &h.authority, nil, "")
}
func (h *httpsChecker) OnPrepare() {
	// TODO
}

func (h *httpsChecker) Handle(req Request, resp Response) (handled bool) {
	if req.IsHTTPS() {
		return false
	}
	// Not https, redirect it.
	if h.permanent {
		resp.SetStatus(StatusMovedPermanently)
	} else {
		resp.SetStatus(StatusFound)
	}
	resp.AddHTTPSRedirection(h.authority)
	resp.SendBytes(nil)
	return true
}
