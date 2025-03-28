// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Limit handlets limit clients' visiting frequency.

package limit

import (
	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterHandlet("limitChecker", func(compName string, stage *Stage, webapp *Webapp) Handlet {
		h := new(limitChecker)
		h.onCreate(compName, stage, webapp)
		return h
	})
}

// limitChecker
type limitChecker struct {
	// Parent
	Handlet_
	// States
}

func (h *limitChecker) onCreate(compName string, stage *Stage, webapp *Webapp) {
	h.Handlet_.OnCreate(compName, stage, webapp)
}
func (h *limitChecker) OnShutdown() { h.Webapp().DecHandlet() }

func (h *limitChecker) OnConfigure() {
	// TODO
}
func (h *limitChecker) OnPrepare() {
	// TODO
}

func (h *limitChecker) Handle(req ServerRequest, resp ServerResponse) (handled bool) {
	// TODO
	return false
}
