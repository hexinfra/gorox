// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Hostname handlets redirect clients to another hostname.

package hostname

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterHandlet("hostnameHandlet", func(name string, stage *Stage, app *App) Handlet {
		h := new(hostnameHandlet)
		h.onCreate(name, stage, app)
		return h
	})
}

// hostnameHandlet
type hostnameHandlet struct {
	// Mixins
	Handlet_
	// Assocs
	stage *Stage
	app   *App
	// States
	hostname  string
	permanent bool
}

func (h *hostnameHandlet) onCreate(name string, stage *Stage, app *App) {
	h.CompInit(name)
	h.stage = stage
	h.app = app
}

func (h *hostnameHandlet) OnConfigure() {
	// hostname
	if v, ok := h.Find("hostname"); ok {
		if hostname, ok := v.String(); ok {
			h.hostname = hostname
		} else {
			UseExitln("invalid hostname")
		}
	} else {
		UseExitln("hostname is required for hostnameHandlet")
	}
	// permanent
	h.ConfigureBool("permanent", &h.permanent, false)
}
func (h *hostnameHandlet) OnPrepare() {
}

func (h *hostnameHandlet) OnShutdown() {
	h.app.SubDone()
}

func (h *hostnameHandlet) Handle(req Request, resp Response) (next bool) {
	if req.Hostname() == h.hostname {
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
