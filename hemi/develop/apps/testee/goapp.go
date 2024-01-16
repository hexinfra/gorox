// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

package testee

import (
	"github.com/hexinfra/gorox/hemi/contrib/routers/simple"

	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterWebappInit("testee", func(webapp *Webapp) error {
		return nil
	})
}

func init() {
	RegisterHandlet("testeeHandlet", func(name string, stage *Stage, webapp *Webapp) Handlet {
		h := new(testeeHandlet)
		h.onCreate(name, stage, webapp)
		return h
	})
}

// testeeHandlet
type testeeHandlet struct {
	// Mixins
	Handlet_
	// Assocs
	stage  *Stage
	webapp *Webapp
	// States
}

func (h *testeeHandlet) onCreate(name string, stage *Stage, webapp *Webapp) {
	h.MakeComp(name)
	h.stage = stage
	h.webapp = webapp
}
func (h *testeeHandlet) OnShutdown() {
	h.webapp.SubDone()
}

func (h *testeeHandlet) OnConfigure() {
}
func (h *testeeHandlet) OnPrepare() {
	r := simple.New()

	h.UseRouter(h, r)
}

func (h *testeeHandlet) Handle(req Request, resp Response) (handled bool) {
	h.Dispatch(req, resp, h.notFound)
	return true
}
func (h *testeeHandlet) notFound(req Request, resp Response) {
	resp.Send("handle not found!")
}
