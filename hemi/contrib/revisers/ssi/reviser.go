// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
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
		r.init(name, stage, app)
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

func (r *ssiReviser) init(name string, stage *Stage, app *App) {
	r.SetName(name)
	r.stage = stage
	r.app = app
}

func (r *ssiReviser) OnConfigure() {
	// rank
	r.ConfigureInt8("rank", &r.rank, func(value int8) bool { return value >= 0 && value < 16 }, RankSSI)
}
func (r *ssiReviser) OnPrepare() {
}
func (r *ssiReviser) OnShutdown() {
}

func (r *ssiReviser) Rank() int8 { return r.rank }

func (r *ssiReviser) BeforeSend(req Request, resp Response) { // identity
	// TODO
}
func (r *ssiReviser) BeforePush(req Request, resp Response) { // chunked
	// TODO
}
func (r *ssiReviser) FinishPush(req Request, resp Response) { // chunked
	// TODO
}

func (r *ssiReviser) Revise(req Request, resp Response, chain Chain) Chain {
	return chain
}