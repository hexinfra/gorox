// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Head revisers can change response head.

package head

import (
	. "github.com/hexinfra/gorox/hemi/contrib/revisers"
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterReviser("headReviser", func(name string, stage *Stage, app *App) Reviser {
		r := new(headReviser)
		r.init(name, stage, app)
		return r
	})
}

// headReviser
type headReviser struct {
	// Mixins
	Reviser_
	// Assocs
	stage *Stage
	app   *App
	// States
	addHeaders map[string]string
	delHeaders []string
}

func (r *headReviser) init(name string, stage *Stage, app *App) {
	r.SetName(name)
	r.stage = stage
	r.app = app
}

func (r *headReviser) OnConfigure() {
	// addHeaders
	r.ConfigureStringDict("addHeaders", &r.addHeaders, nil, map[string]string{"date": "Mon, 14 Sep 2015 03:19:14 GMT"})
	// delHeaders
	r.ConfigureStringList("delHeaders", &r.delHeaders, nil, []string{})
}
func (r *headReviser) OnPrepare() {
}
func (r *headReviser) OnShutdown() {
	r.app.SubDone()
}

func (r *headReviser) Rank() int8 { return RankHead }

func (r *headReviser) BeforeRecv(req Request, resp Response) { // identity
	// TODO
}

func (r *headReviser) BeforePull(req Request, resp Response) { // chunked
	// TODO
}
func (r *headReviser) FinishPull(req Request, resp Response) { // chunked
	// TODO
}

func (r *headReviser) Change(req Request, resp Response, chain Chain) Chain {
	return chain
}

func (r *headReviser) BeforeSend(req Request, resp Response) { // identity
	// TODO
}
func (r *headReviser) BeforePush(req Request, resp Response) { // chunked
	// TODO
}
func (r *headReviser) FinishPush(req Request, resp Response) { // chunked
	// TODO
}

func (r *headReviser) Revise(req Request, resp Response, chain Chain) Chain {
	// Do nothing.
	return chain
}
