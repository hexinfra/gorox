// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Limit handlets limit clients' visiting frequency.

package limit

import (
	. "github.com/hexinfra/gorox/hemi/internal"
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
	// Mixins
	Handlet_
	// Assocs
	stage  *Stage
	webapp *Webapp
	// States
}

func (h *limitChecker) onCreate(name string, stage *Stage, webapp *Webapp) {
	h.MakeComp(name)
	h.stage = stage
	h.webapp = webapp
}
func (h *limitChecker) OnShutdown() {
	h.webapp.SubDone()
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
