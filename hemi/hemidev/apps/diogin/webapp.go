// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

package diogin

import (
	"github.com/hexinfra/gorox/hemi/contrib/mappers/simple"

	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterWebappInit("diogin", func(webapp *Webapp) error {
		return nil
	})
	RegisterHandlet("dioginHandlet", func(name string, stage *Stage, webapp *Webapp) Handlet {
		h := new(dioginHandlet)
		h.onCreate(name, stage, webapp)
		return h
	})
}

// dioginHandlet
type dioginHandlet struct {
	// Parent
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
	h.webapp.DecSub() // handlet
}

func (h *dioginHandlet) OnConfigure() {
}
func (h *dioginHandlet) OnPrepare() {
	m := simple.New()

	h.UseMapper(h, m)
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
