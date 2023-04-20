// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
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
	r.MakeComp(name)
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
	// TODO
}

func (r *headReviser) Rank() int8 { return RankHead }

func (r *headReviser) BeforeRecv(req Request, resp Response) { // sized
	// TODO
}
func (r *headReviser) OnRecv(req Request, resp Response, chain Chain) (Chain, bool) {
	return chain, true
}

func (r *headReviser) BeforeSend(req Request, resp Response) { // sized
	// TODO
}
func (r *headReviser) OnSend(req Request, resp Response, content *Chain) {
}

func (r *headReviser) BeforeDraw(req Request, resp Response) { // unsized
	// TODO
}
func (r *headReviser) OnDraw(req Request, resp Response, chain Chain) (Chain, bool) {
	return chain, true
}
func (r *headReviser) FinishDraw(req Request, resp Response) { // unsized
	// TODO
}

func (r *headReviser) BeforeEcho(req Request, resp Response) { // unsized
	// TODO
}
func (r *headReviser) OnEcho(req Request, resp Response, chunks *Chain) {
}
func (r *headReviser) FinishEcho(req Request, resp Response) { // unsized
	// TODO
}
