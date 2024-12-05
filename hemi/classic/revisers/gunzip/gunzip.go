// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
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
	RegisterReviser("gunzipReviser", func(name string, stage *Stage, webapp *Webapp) Reviser {
		r := new(gunzipReviser)
		r.onCreate(name, stage, webapp)
		return r
	})
}

// gunzipReviser
type gunzipReviser struct {
	// Parent
	Reviser_
	// Assocs
	stage  *Stage // current stage
	webapp *Webapp
	// States
	onContentTypes []string
}

func (r *gunzipReviser) onCreate(name string, stage *Stage, webapp *Webapp) {
	r.MakeComp(name)
	r.stage = stage
	r.webapp = webapp
}
func (r *gunzipReviser) OnShutdown() {
	r.webapp.DecSub() // reviser
}

func (r *gunzipReviser) OnConfigure() {
	// onContentTypes
	r.ConfigureStringList("onContentTypes", &r.onContentTypes, nil, []string{"text/html"})
}
func (r *gunzipReviser) OnPrepare() {
	// TODO
}

func (r *gunzipReviser) Rank() int8 { return RankGunzip }

func (r *gunzipReviser) BeforeRecv(req Request, resp Response) { // sized
	// TODO
}
func (r *gunzipReviser) BeforeDraw(req Request, resp Response) { // vague
	// TODO
}
func (r *gunzipReviser) OnInput(req Request, resp Response, chain *Chain) bool {
	// TODO
	return true
}
func (r *gunzipReviser) FinishDraw(req Request, resp Response) { // vague
	// TODO
}

func (r *gunzipReviser) BeforeSend(req Request, resp Response) { // sized
	// TODO
}
func (r *gunzipReviser) BeforeEcho(req Request, resp Response) { // vague
	// TODO
}
func (r *gunzipReviser) OnOutput(req Request, resp Response, chain *Chain) {
	// TODO
}
func (r *gunzipReviser) FinishEcho(req Request, resp Response) { // vague
	// TODO
}
