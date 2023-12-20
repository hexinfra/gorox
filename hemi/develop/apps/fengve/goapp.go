// Copyright (c) 2020-2023 Feng Wei <feng19910104@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

package fengve

import (
	"github.com/hexinfra/gorox/hemi/contrib/routers/simple"

	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterWebappInit("fengve", func(webapp *Webapp) error {
		return nil
	})
}

func init() {
	RegisterHandlet("fengveHandlet", func(name string, stage *Stage, webapp *Webapp) Handlet {
		h := new(fengveHandlet)
		h.onCreate(name, stage, webapp)
		return h
	})
}

// fengveHandlet
type fengveHandlet struct {
	// Mixins
	Handlet_
	// Assocs
	stage  *Stage
	webapp *Webapp
	// States
}

func (h *fengveHandlet) onCreate(name string, stage *Stage, webapp *Webapp) {
	h.MakeComp(name)
	h.stage = stage
	h.webapp = webapp
}
func (h *fengveHandlet) OnShutdown() {
	h.webapp.SubDone()
}

func (h *fengveHandlet) OnConfigure() {
}
func (h *fengveHandlet) OnPrepare() {
	r := simple.New()
	r.GET("/abc", h.abc)

	h.UseRouter(h, r)
}

func (h *fengveHandlet) Handle(req Request, resp Response) (handled bool) {
	h.Dispatch(req, resp, h.notFound)
	return true
}
func (h *fengveHandlet) notFound(req Request, resp Response) {
	resp.Send("handle not found!")
}
func (h *fengveHandlet) abc(req Request, resp Response) {
	resp.Send("abc")
}
