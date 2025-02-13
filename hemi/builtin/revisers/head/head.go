// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Head revisers can change response head.

package head

import (
	. "github.com/hexinfra/gorox/hemi"
	. "github.com/hexinfra/gorox/hemi/builtin/revisers"
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
func (r *headReviser) OnShutdown() { r.Webapp().DecReviser() }

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

func (r *headReviser) BeforeRecv(req ServerRequest, resp ServerResponse) { // sized
	// TODO
}
func (r *headReviser) BeforeDraw(req ServerRequest, resp ServerResponse) { // vague
	// TODO
}
func (r *headReviser) OnInput(req ServerRequest, resp ServerResponse, input *Chain) bool {
	// TODO
	return true
}
func (r *headReviser) FinishDraw(req ServerRequest, resp ServerResponse) { // vague
	// TODO
}

func (r *headReviser) BeforeSend(req ServerRequest, resp ServerResponse) { // sized
	// TODO
}
func (r *headReviser) BeforeEcho(req ServerRequest, resp ServerResponse) { // vague
	// TODO
}
func (r *headReviser) OnOutput(req ServerRequest, resp ServerResponse, output *Chain) {
	// TODO
}
func (r *headReviser) FinishEcho(req ServerRequest, resp ServerResponse) { // vague
	// TODO
}
