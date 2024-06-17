// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Limit handlets limit clients' visiting frequency.

package limit

import (
	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterHandlet("limitChecker", func(name string, stage *Stage, webapp *Webapp) Handlet {
		h := new(limitChecker)
		h.onCreate(name, stage, webapp)
		return h
	})
}

// limitChecker
type limitChecker struct {
	// Parent
	Handlet_
	// Assocs
	stage  *Stage // current stage
	webapp *Webapp
	// States
}

func (h *limitChecker) onCreate(name string, stage *Stage, webapp *Webapp) {
	h.MakeComp(name)
	h.stage = stage
	h.webapp = webapp
}
func (h *limitChecker) OnShutdown() {
	h.webapp.DecSub() // handlet
}

func (h *limitChecker) OnConfigure() {
	// TODO
}
func (h *limitChecker) OnPrepare() {
	// TODO
}

func (h *limitChecker) Handle(req Request, resp Response) (handled bool) {
	// TODO
	return false
}
