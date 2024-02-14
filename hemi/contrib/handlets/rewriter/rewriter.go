// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Rewriter checkers rewrite request path.

package rewriter

import (
	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterHandlet("rewriter", func(name string, stage *Stage, webapp *Webapp) Handlet {
		h := new(rewriterChecker)
		h.onCreate(name, stage, webapp)
		return h
	})
}

// rewriterChecker
type rewriterChecker struct {
	// Mixins
	Handlet_
	// Assocs
	stage  *Stage // current stage
	webapp *Webapp
	// States
}

func (h *rewriterChecker) onCreate(name string, stage *Stage, webapp *Webapp) {
	h.MakeComp(name)
	h.stage = stage
	h.webapp = webapp
}
func (h *rewriterChecker) OnShutdown() {
	h.webapp.SubDone()
}

func (h *rewriterChecker) OnConfigure() {
	// TODO
}
func (h *rewriterChecker) OnPrepare() {
	// TODO
}

func (h *rewriterChecker) Handle(req Request, resp Response) (handled bool) {
	// TODO
	return false
}
