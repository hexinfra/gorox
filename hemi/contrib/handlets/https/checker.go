// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HTTPS checkers redirect request urls to its HTTPS version.

package https

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterHandlet("httpsChecker", func(name string, stage *Stage, app *App) Handlet {
		h := new(httpsChecker)
		h.onCreate(name, stage, app)
		return h
	})
}

// httpsChecker
type httpsChecker struct {
	// Mixins
	Handlet_
	// Assocs
	stage *Stage
	app   *App
	// States
	permanent bool
	authority string
}

func (h *httpsChecker) onCreate(name string, stage *Stage, app *App) {
	h.MakeComp(name)
	h.stage = stage
	h.app = app
}
func (h *httpsChecker) OnShutdown() {
	h.app.SubDone()
}

func (h *httpsChecker) OnConfigure() {
	// permanent
	h.ConfigureBool("permanent", &h.permanent, false)
	// authority
	h.ConfigureString("authority", &h.authority, nil, "")
}
func (h *httpsChecker) OnPrepare() {
}

func (h *httpsChecker) Handle(req Request, resp Response) (next bool) {
	if req.IsHTTPS() {
		return true
	}
	if h.permanent {
		resp.SetStatus(StatusMovedPermanently)
	} else {
		resp.SetStatus(StatusFound)
	}
	resp.AddHTTPSRedirection(h.authority)
	resp.SendBytes(nil)
	return
}
