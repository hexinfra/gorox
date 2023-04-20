// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Wrap revisers add something before or after response content.

package wrap

import (
	. "github.com/hexinfra/gorox/hemi/contrib/revisers"
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterReviser("wrapReviser", func(name string, stage *Stage, app *App) Reviser {
		r := new(wrapReviser)
		r.onCreate(name, stage, app)
		return r
	})
}

// wrapReviser
type wrapReviser struct {
	// Mixins
	Reviser_
	// Assocs
	stage *Stage
	app   *App
	// States
	rank int8
}

func (r *wrapReviser) onCreate(name string, stage *Stage, app *App) {
	r.MakeComp(name)
	r.stage = stage
	r.app = app
}
func (r *wrapReviser) OnShutdown() {
	r.app.SubDone()
}

func (r *wrapReviser) OnConfigure() {
	// rank
	r.ConfigureInt8("rank", &r.rank, func(value int8) bool { return value >= 0 && value < 16 }, RankWrap)
}
func (r *wrapReviser) OnPrepare() {
	// TODO
}

func (r *wrapReviser) Rank() int8 { return r.rank }

func (r *wrapReviser) BeforeRecv(req Request, resp Response) { // sized
	// TODO
}
func (r *wrapReviser) OnRecv(req Request, resp Response, chain Chain) (Chain, bool) {
	return chain, true
}

func (r *wrapReviser) BeforeSend(req Request, resp Response) { // sized
	// TODO
}
func (r *wrapReviser) OnSend(req Request, resp Response, content *Chain) {
	if IsDebug(2) {
		block := GetBlock()
		block.SetText([]byte("d"))
		content.PushTail(block)
	}
}

func (r *wrapReviser) BeforeDraw(req Request, resp Response) { // unsized
	// TODO
}
func (r *wrapReviser) OnDraw(req Request, resp Response, chain Chain) (Chain, bool) {
	return chain, true
}
func (r *wrapReviser) FinishDraw(req Request, resp Response) { // unsized
	// TODO
}

func (r *wrapReviser) BeforeEcho(req Request, resp Response) { // unsized
	// TODO
	if IsDebug(2) {
		Debugln("BeforeEcho")
	}
}
func (r *wrapReviser) OnEcho(req Request, resp Response, chunks *Chain) {
	if IsDebug(2) {
		block := GetBlock()
		block.SetText([]byte("c"))
		chunks.PushTail(block)
	}
}
func (r *wrapReviser) FinishEcho(req Request, resp Response) { // unsized
	// TODO
	if IsDebug(2) {
		Debugln("FinishEcho")
	}
}
