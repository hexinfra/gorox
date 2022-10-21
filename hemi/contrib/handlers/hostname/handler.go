// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Hostname handlers redirect clients to another hostname.

package hostname

import (
	"bytes"
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterHandler("hostnameHandler", func(name string, stage *Stage, app *App) Handler {
		h := new(hostnameHandler)
		h.init(name, stage, app)
		return h
	})
}

// hostnameHandler
type hostnameHandler struct {
	// Mixins
	Handler_
	// Assocs
	stage *Stage
	app   *App
	// States
	hostname  []byte
	permanent bool
}

func (h *hostnameHandler) init(name string, stage *Stage, app *App) {
	h.SetName(name)
	h.stage = stage
	h.app = app
}

func (h *hostnameHandler) OnConfigure() {
	// hostname
	if v, ok := h.Find("hostname"); ok {
		if hostname, ok := v.String(); ok {
			h.hostname = []byte(hostname)
		} else {
			UseExitln("invalid hostname")
		}
	} else {
		UseExitln("hostname is required for hostnameHandler")
	}
	// permanent
	h.ConfigureBool("permanent", &h.permanent, false)
}
func (h *hostnameHandler) OnPrepare() {
}
func (h *hostnameHandler) OnShutdown() {
}

func (h *hostnameHandler) Handle(req Request, resp Response) (next bool) {
	if bytes.Equal(req.UnsafeHostname(), h.hostname) {
		return true
	}
	if h.permanent {
		resp.SetStatus(StatusMovedPermanently)
	} else {
		resp.SetStatus(StatusFound)
	}
	resp.AddHostnameRedirection(h.hostname)
	resp.SendBytes(nil)
	return
}
