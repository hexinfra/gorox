// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Gunzip revisers can gunzip response content.

package gunzip

import (
	. "github.com/hexinfra/gorox/hemi/contrib/revisers"
	. "github.com/hexinfra/gorox/hemi/internal"
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
	// Mixins
	Reviser_
	// Assocs
	stage  *Stage
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
	r.webapp.SubDone()
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
func (r *gunzipReviser) OnRecv(req Request, resp Response, chain Chain) (Chain, bool) { // sized
	return chain, true
}

func (r *gunzipReviser) BeforeSend(req Request, resp Response) { // sized
	// TODO
}
func (r *gunzipReviser) OnSend(req Request, resp Response, content *Chain) { // sized
}

func (r *gunzipReviser) BeforeDraw(req Request, resp Response) { // unsized
	// TODO
}
func (r *gunzipReviser) OnDraw(req Request, resp Response, chain Chain) (Chain, bool) { // unsized
	return chain, true
}
func (r *gunzipReviser) FinishDraw(req Request, resp Response) { // unsized
	// TODO
}

func (r *gunzipReviser) BeforeEcho(req Request, resp Response) { // unsized
	// TODO
}
func (r *gunzipReviser) OnEcho(req Request, resp Response, chunks *Chain) { // unsized
}
func (r *gunzipReviser) FinishEcho(req Request, resp Response) { // unsized
	// TODO
}
