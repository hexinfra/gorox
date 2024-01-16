// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

package diogin

import (
	"github.com/hexinfra/gorox/hemi/contrib/routers/simple"

	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterWebappInit("diogin", func(webapp *Webapp) error {
		return nil
	})
}

func init() {
	RegisterHandlet("dioginHandlet", func(name string, stage *Stage, webapp *Webapp) Handlet {
		h := new(dioginHandlet)
		h.onCreate(name, stage, webapp)
		return h
	})
}

// dioginHandlet
type dioginHandlet struct {
	// Mixins
	Handlet_
	// Assocs
	stage  *Stage
	webapp *Webapp
	// States
}

func (h *dioginHandlet) onCreate(name string, stage *Stage, webapp *Webapp) {
	h.MakeComp(name)
	h.stage = stage
	h.webapp = webapp
}
func (h *dioginHandlet) OnShutdown() {
	h.webapp.SubDone()
}

func (h *dioginHandlet) OnConfigure() {
}
func (h *dioginHandlet) OnPrepare() {
	r := simple.New()

	h.UseRouter(h, r)
}

func (h *dioginHandlet) Handle(req Request, resp Response) (handled bool) {
	h.Dispatch(req, resp, h.notFound)
	return true
}
func (h *dioginHandlet) notFound(req Request, resp Response) {
	resp.Send("handle not found!")
}

func (h *dioginHandlet) GET_(req Request, resp Response) {
	resp.Send("diogin's index page")
}
func (h *dioginHandlet) GET_send(req Request, resp Response) {
	resp.Send("utf-8中文字符串")
}
func (h *dioginHandlet) GET_echo(req Request, resp Response) {
	resp.Echo("a")
	resp.Echo("b")
}
