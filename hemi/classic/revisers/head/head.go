// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Head revisers can change response head.

package head

import (
	. "github.com/hexinfra/gorox/hemi"
	. "github.com/hexinfra/gorox/hemi/classic/revisers"
)

func init() {
	RegisterReviser("headReviser", func(compName string, stage *Stage, webapp *Webapp) Reviser {
		r := new(headReviser)
		r.onCreate(compName, stage, webapp)
		return r
	})
}

// headReviser
type headReviser struct {
	// Parent
	Reviser_
	// States
	addRequest  map[string]string
	delRequest  []string
	addResponse map[string]string
	delResponse []string
}

func (r *headReviser) onCreate(compName string, stage *Stage, webapp *Webapp) {
	r.Reviser_.OnCreate(compName, stage, webapp)
}
func (r *headReviser) OnShutdown() {
	r.Webapp().DecSub() // reviser
}

func (r *headReviser) OnConfigure() {
	// .addRequest
	r.ConfigureStringDict("addRequest", &r.addRequest, nil, map[string]string{"date": "Mon, 14 Sep 2015 03:19:14 GMT"})
	// .delRequest
	r.ConfigureStringList("delRequest", &r.delRequest, nil, []string{})
	// .addResponse
	r.ConfigureStringDict("addResponse", &r.addResponse, nil, map[string]string{"date": "Mon, 14 Sep 2015 03:19:14 GMT"})
	// .delResponse
	r.ConfigureStringList("delResponse", &r.delResponse, nil, []string{})
}
func (r *headReviser) OnPrepare() {
	// TODO
}

func (r *headReviser) Rank() int8 { return RankHead }

func (r *headReviser) BeforeRecv(req Request, resp Response) { // sized
	// TODO
}
func (r *headReviser) BeforeDraw(req Request, resp Response) { // vague
	// TODO
}
func (r *headReviser) OnInput(req Request, resp Response, input *Chain) bool {
	// TODO
	return true
}
func (r *headReviser) FinishDraw(req Request, resp Response) { // vague
	// TODO
}

func (r *headReviser) BeforeSend(req Request, resp Response) { // sized
	// TODO
}
func (r *headReviser) BeforeEcho(req Request, resp Response) { // vague
	// TODO
}
func (r *headReviser) OnOutput(req Request, resp Response, output *Chain) {
	// TODO
}
func (r *headReviser) FinishEcho(req Request, resp Response) { // vague
	// TODO
}
