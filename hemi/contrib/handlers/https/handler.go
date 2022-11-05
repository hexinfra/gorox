// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Scheme handlers redirect request urls to its HTTPS version.

package https

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterHandler("httpsHandler", func(name string, stage *Stage, app *App) Handler {
		h := new(httpsHandler)
		h.init(name, stage, app)
		return h
	})
}

// httpsHandler
type httpsHandler struct {
	// Mixins
	Handler_
	// Assocs
	stage *Stage
	app   *App
	// States
	permanent bool
	authority string
}

func (h *httpsHandler) init(name string, stage *Stage, app *App) {
	h.Handler_.Init(name, h)
	h.stage = stage
	h.app = app
}

func (h *httpsHandler) OnConfigure() {
	// permanent
	h.ConfigureBool("permanent", &h.permanent, false)
	// authority
	h.ConfigureString("authority", &h.authority, nil, "")
}
func (h *httpsHandler) OnPrepare() {
}
func (h *httpsHandler) OnShutdown() {
}

func (h *httpsHandler) Handle(req Request, resp Response) (next bool) {
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
