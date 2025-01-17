// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Hostname checkers redirect clients to another hostname.

package hostname

import (
	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterHandlet("hostnameChecker", func(compName string, stage *Stage, webapp *Webapp) Handlet {
		h := new(hostnameChecker)
		h.onCreate(compName, stage, webapp)
		return h
	})
}

// hostnameChecker
type hostnameChecker struct {
	// Parent
	Handlet_
	// States
	hostname  string
	permanent bool
}

func (h *hostnameChecker) onCreate(compName string, stage *Stage, webapp *Webapp) {
	h.Handlet_.OnCreate(compName, stage, webapp)
}
func (h *hostnameChecker) OnShutdown() {
	h.Webapp().DecSub() // handlet
}

func (h *hostnameChecker) OnConfigure() {
	// .hostname
	if v, ok := h.Find("hostname"); ok {
		if hostname, ok := v.String(); ok {
			h.hostname = hostname
		} else {
			UseExitln("invalid hostname")
		}
	} else {
		UseExitln("hostname is required for hostnameChecker")
	}

	// .permanent
	h.ConfigureBool("permanent", &h.permanent, false)
}
func (h *hostnameChecker) OnPrepare() {
	// TODO
}

func (h *hostnameChecker) Handle(req Request, resp Response) (handled bool) {
	if req.Hostname() == h.hostname {
		return false
	}
	// Not hostname, redirect it.
	if h.permanent {
		resp.SetStatus(StatusMovedPermanently)
	} else {
		resp.SetStatus(StatusFound)
	}
	resp.AddHostnameRedirection(h.hostname)
	resp.SendBytes(nil)
	return true
}
