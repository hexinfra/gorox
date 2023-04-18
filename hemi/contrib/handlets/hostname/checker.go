// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Hostname checkers redirect clients to another hostname.

package hostname

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterHandlet("hostnameChecker", func(name string, stage *Stage, app *App) Handlet {
		h := new(hostnameChecker)
		h.onCreate(name, stage, app)
		return h
	})
}

// hostnameChecker
type hostnameChecker struct {
	// Mixins
	Handlet_
	// Assocs
	stage *Stage
	app   *App
	// States
	hostname  string
	permanent bool
}

func (h *hostnameChecker) onCreate(name string, stage *Stage, app *App) {
	h.MakeComp(name)
	h.stage = stage
	h.app = app
}
func (h *hostnameChecker) OnShutdown() {
	h.app.SubDone()
}

func (h *hostnameChecker) OnConfigure() {
	// hostname
	if v, ok := h.Find("hostname"); ok {
		if hostname, ok := v.String(); ok {
			h.hostname = hostname
		} else {
			UseExitln("invalid hostname")
		}
	} else {
		UseExitln("hostname is required for hostnameChecker")
	}
	// permanent
	h.ConfigureBool("permanent", &h.permanent, false)
}
func (h *hostnameChecker) OnPrepare() {
	// TODO
}

func (h *hostnameChecker) Handle(req Request, resp Response) (next bool) {
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
