// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

package hemiapp

import (
	"github.com/hexinfra/gorox/hemi/builtin/mappers/simple"

	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterWebappInit("hemiapp", func(webapp *Webapp) error {
		return nil
	})
}

func init() {
	RegisterHandlet("hemiappHandlet", func(compName string, stage *Stage, webapp *Webapp) Handlet {
		h := new(hemiappHandlet)
		h.onCreate(compName, stage, webapp)
		return h
	})
}

// hemiappHandlet
type hemiappHandlet struct {
	// Parent
	Handlet_
	// States
}

func (h *hemiappHandlet) onCreate(compName string, stage *Stage, webapp *Webapp) {
	h.Handlet_.OnCreate(compName, stage, webapp)
}
func (h *hemiappHandlet) OnShutdown() {
	h.Webapp().DecSub() // handlet
}

func (h *hemiappHandlet) OnConfigure() {
}
func (h *hemiappHandlet) OnPrepare() {
	m := simple.New()

	h.UseMapper(h, m)
}

func (h *hemiappHandlet) Handle(req ServerRequest, resp ServerResponse) (handled bool) {
	h.Dispatch(req, resp, h.notFound)
	return true
}
func (h *hemiappHandlet) notFound(req ServerRequest, resp ServerResponse) {
	resp.Send("404 handle not found!")
}
