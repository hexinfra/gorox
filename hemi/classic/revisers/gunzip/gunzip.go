// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Gunzip revisers can gunzip response content.

package gunzip

import (
	. "github.com/hexinfra/gorox/hemi"
	. "github.com/hexinfra/gorox/hemi/classic/revisers"
)

func init() {
	RegisterReviser("gunzipReviser", func(compName string, stage *Stage, webapp *Webapp) Reviser {
		r := new(gunzipReviser)
		r.onCreate(compName, stage, webapp)
		return r
	})
}

// gunzipReviser
type gunzipReviser struct {
	// Parent
	Reviser_
	// States
	onContentTypes []string
}

func (r *gunzipReviser) onCreate(compName string, stage *Stage, webapp *Webapp) {
	r.Reviser_.OnCreate(compName, stage, webapp)
}
func (r *gunzipReviser) OnShutdown() {
	r.Webapp().DecSub() // reviser
}

func (r *gunzipReviser) OnConfigure() {
	// .onContentTypes
	r.ConfigureStringList("onContentTypes", &r.onContentTypes, nil, []string{"text/html"})
}
func (r *gunzipReviser) OnPrepare() {
	// TODO
}

func (r *gunzipReviser) Rank() int8 { return RankGunzip }

func (r *gunzipReviser) BeforeRecv(req ServerRequest, resp ServerResponse) { // sized
	// TODO
}
func (r *gunzipReviser) BeforeDraw(req ServerRequest, resp ServerResponse) { // vague
	// TODO
}
func (r *gunzipReviser) OnInput(req ServerRequest, resp ServerResponse, input *Chain) bool {
	// TODO
	return true
}
func (r *gunzipReviser) FinishDraw(req ServerRequest, resp ServerResponse) { // vague
	// TODO
}

func (r *gunzipReviser) BeforeSend(req ServerRequest, resp ServerResponse) { // sized
	// TODO
}
func (r *gunzipReviser) BeforeEcho(req ServerRequest, resp ServerResponse) { // vague
	// TODO
}
func (r *gunzipReviser) OnOutput(req ServerRequest, resp ServerResponse, output *Chain) {
	// TODO
}
func (r *gunzipReviser) FinishEcho(req ServerRequest, resp ServerResponse) { // vague
	// TODO
}
