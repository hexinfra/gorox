// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// HTTPS checkers redirect request urls to its HTTPS version.

package https

import (
	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterHandlet("httpsChecker", func(compName string, stage *Stage, webapp *Webapp) Handlet {
		h := new(httpsChecker)
		h.onCreate(compName, stage, webapp)
		return h
	})
}

// httpsChecker
type httpsChecker struct {
	// Parent
	Handlet_
	// States
	permanent bool
	authority string
}

func (h *httpsChecker) onCreate(compName string, stage *Stage, webapp *Webapp) {
	h.Handlet_.OnCreate(compName, stage, webapp)
}
func (h *httpsChecker) OnShutdown() { h.Webapp().DecHandlet() }

func (h *httpsChecker) OnConfigure() {
	// .permanent
	h.ConfigureBool("permanent", &h.permanent, false)
	// .authority
	h.ConfigureString("authority", &h.authority, nil, "")
}
func (h *httpsChecker) OnPrepare() {
	// TODO
}

func (h *httpsChecker) Handle(req ServerRequest, resp ServerResponse) (handled bool) {
	if req.IsHTTPS() {
		return false
	}
	// Not https, redirect it.
	if h.permanent { // 301
		resp.SetStatus(StatusMovedPermanently)
	} else { // 302
		resp.SetStatus(StatusFound)
	}
	resp.AddHTTPSRedirection(h.authority)
	resp.SendBytes(nil)
	return true
}
