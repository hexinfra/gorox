// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Replace revisers replace something in response content.

package replace

import (
	. "github.com/hexinfra/gorox/hemi/internal"
	. "github.com/hexinfra/gorox/hemi/standard/revisers"
)

func init() {
	RegisterReviser("replaceReviser", func(name string, stage *Stage, app *App) Reviser {
		r := new(replaceReviser)
		r.onCreate(name, stage, app)
		return r
	})
}

// replaceReviser
type replaceReviser struct {
	// Mixins
	Reviser_
	// Assocs
	stage *Stage
	app   *App
	// States
	rank int8
}

func (r *replaceReviser) onCreate(name string, stage *Stage, app *App) {
	r.CompInit(name)
	r.stage = stage
	r.app = app
}
func (r *replaceReviser) OnShutdown() {
	r.app.SubDone()
}

func (r *replaceReviser) OnConfigure() {
	// rank
	r.ConfigureInt8("rank", &r.rank, func(value int8) bool { return value >= 0 && value < 16 }, RankReplace)
}
func (r *replaceReviser) OnPrepare() {
}

func (r *replaceReviser) Rank() int8 { return r.rank }

func (r *replaceReviser) BeforeRecv(req Request, resp Response) { // counted
	// TODO
}
func (r *replaceReviser) BeforePull(req Request, resp Response) { // chunked
	// TODO
}
func (r *replaceReviser) FinishPull(req Request, resp Response) { // chunked
	// TODO
}
func (r *replaceReviser) OnInput(req Request, resp Response, chain Chain) (Chain, bool) {
	return chain, true
}

func (r *replaceReviser) BeforeSend(req Request, resp Response) { // counted
	// TODO
}
func (r *replaceReviser) BeforePush(req Request, resp Response) { // chunked
	// TODO
}
func (r *replaceReviser) FinishPush(req Request, resp Response) { // chunked
	// TODO
}
func (r *replaceReviser) OnOutput(req Request, resp Response, chain Chain) Chain {
	return chain
}
