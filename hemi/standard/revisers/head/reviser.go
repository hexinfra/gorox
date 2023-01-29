// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Head revisers can change response head.

package head

import (
	. "github.com/hexinfra/gorox/hemi/internal"
	. "github.com/hexinfra/gorox/hemi/standard/revisers"
)

func init() {
	RegisterReviser("headReviser", func(name string, stage *Stage, app *App) Reviser {
		r := new(headReviser)
		r.onCreate(name, stage, app)
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
	addRequest  map[string]string
	delRequest  []string
	addResponse map[string]string
	delResponse []string
}

func (r *headReviser) onCreate(name string, stage *Stage, app *App) {
	r.CompInit(name)
	r.stage = stage
	r.app = app
}
func (r *headReviser) OnShutdown() {
	r.app.SubDone()
}

func (r *headReviser) OnConfigure() {
	// addRequest
	r.ConfigureStringDict("addRequest", &r.addRequest, nil, map[string]string{"date": "Mon, 14 Sep 2015 03:19:14 GMT"})
	// delRequest
	r.ConfigureStringList("delRequest", &r.delRequest, nil, []string{})
	// addResponse
	r.ConfigureStringDict("addResponse", &r.addResponse, nil, map[string]string{"date": "Mon, 14 Sep 2015 03:19:14 GMT"})
	// delResponse
	r.ConfigureStringList("delResponse", &r.delResponse, nil, []string{})
}
func (r *headReviser) OnPrepare() {
}

func (r *headReviser) Rank() int8 { return RankHead }

func (r *headReviser) BeforeRecv(req Request, resp Response) { // counted
	// TODO
}
func (r *headReviser) BeforePull(req Request, resp Response) { // chunked
	// TODO
}
func (r *headReviser) FinishPull(req Request, resp Response) { // chunked
	// TODO
}
func (r *headReviser) OnInput(req Request, resp Response, chain Chain) Chain {
	return chain
}

func (r *headReviser) BeforeSend(req Request, resp Response) { // counted
	// TODO
}
func (r *headReviser) BeforePush(req Request, resp Response) { // chunked
	// TODO
}
func (r *headReviser) FinishPush(req Request, resp Response) { // chunked
	// TODO
}
func (r *headReviser) OnOutput(req Request, resp Response, chain Chain) Chain {
	// Do nothing.
	return chain
}
