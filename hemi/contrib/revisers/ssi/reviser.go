// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// SSI revisers can include other files in response content.

package ssi

import (
	. "github.com/hexinfra/gorox/hemi/contrib/revisers"
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterReviser("ssiReviser", func(name string, stage *Stage, app *App) Reviser {
		r := new(ssiReviser)
		r.onCreate(name, stage, app)
		return r
	})
}

// ssiReviser
type ssiReviser struct {
	// Mixins
	Reviser_
	// Assocs
	stage *Stage
	app   *App
	// States
	rank int8
}

func (r *ssiReviser) onCreate(name string, stage *Stage, app *App) {
	r.MakeComp(name)
	r.stage = stage
	r.app = app
}
func (r *ssiReviser) OnShutdown() {
	r.app.SubDone()
}

func (r *ssiReviser) OnConfigure() {
	// rank
	r.ConfigureInt8("rank", &r.rank, func(value int8) bool { return value >= 0 && value < 16 }, RankSSI)
}
func (r *ssiReviser) OnPrepare() {
	// TODO
}

func (r *ssiReviser) Rank() int8 { return r.rank }

func (r *ssiReviser) BeforeRecv(req Request, resp Response) { // sized
	// TODO
}
func (r *ssiReviser) OnRecv(req Request, resp Response, chain Chain) (Chain, bool) {
	return chain, true
}

func (r *ssiReviser) BeforeDraw(req Request, resp Response) { // unsized
	// TODO
}
func (r *ssiReviser) OnDraw(req Request, resp Response, chain Chain) (Chain, bool) {
	return chain, true
}
func (r *ssiReviser) FinishDraw(req Request, resp Response) { // unsized
	// TODO
}

func (r *ssiReviser) BeforeSend(req Request, resp Response) { // sized
	// TODO
}
func (r *ssiReviser) OnSend(req Request, resp Response, content *Chain) {
}

func (r *ssiReviser) BeforeEcho(req Request, resp Response) { // unsized
	// TODO
}
func (r *ssiReviser) OnEcho(req Request, resp Response, chunks *Chain) {
}
func (r *ssiReviser) FinishEcho(req Request, resp Response) { // unsized
	// TODO
}
