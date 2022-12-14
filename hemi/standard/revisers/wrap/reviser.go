// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Wrap revisers add something before or after response content.

package wrap

import (
	. "github.com/hexinfra/gorox/hemi/internal"
	. "github.com/hexinfra/gorox/hemi/standard/revisers"
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
	r.CompInit(name)
	r.stage = stage
	r.app = app
}

func (r *wrapReviser) OnConfigure() {
	// rank
	r.ConfigureInt8("rank", &r.rank, func(value int8) bool { return value >= 0 && value < 16 }, RankWrap)
}
func (r *wrapReviser) OnPrepare() {
}

func (r *wrapReviser) OnShutdown() {
	r.app.SubDone()
}

func (r *wrapReviser) Rank() int8 { return r.rank }

func (r *wrapReviser) BeforeRecv(req Request, resp Response) { // sized
	// TODO
}

func (r *wrapReviser) BeforePull(req Request, resp Response) { // chunked
	// TODO
}
func (r *wrapReviser) FinishPull(req Request, resp Response) { // chunked
	// TODO
}

func (r *wrapReviser) Change(req Request, resp Response, chain Chain) Chain {
	return chain
}

func (r *wrapReviser) BeforeSend(req Request, resp Response) { // sized
	// TODO
}
func (r *wrapReviser) BeforePush(req Request, resp Response) { // chunked
	// TODO
}
func (r *wrapReviser) FinishPush(req Request, resp Response) { // chunked
	// TODO
}

func (r *wrapReviser) Revise(req Request, resp Response, chain Chain) Chain {
	return chain
}
