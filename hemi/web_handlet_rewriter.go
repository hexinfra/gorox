// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Rewriters rewrite request path.

// This handlet is currently under design.

package hemi

func init() {
	RegisterHandlet("rewriter", func(compName string, stage *Stage, webapp *Webapp) Handlet {
		h := new(rewriterChecker)
		h.onCreate(compName, stage, webapp)
		return h
	})
}

// rewriterChecker
type rewriterChecker struct {
	// Parent
	Handlet_
	// States
}

func (h *rewriterChecker) onCreate(compName string, stage *Stage, webapp *Webapp) {
	h.Handlet_.OnCreate(compName, stage, webapp)
}
func (h *rewriterChecker) OnShutdown() { h.webapp.DecHandlet() }

func (h *rewriterChecker) OnConfigure() {
	// TODO
}
func (h *rewriterChecker) OnPrepare() {
	// TODO
}

func (h *rewriterChecker) Handle(req ServerRequest, resp ServerResponse) (handled bool) {
	// TODO
	return false
}
