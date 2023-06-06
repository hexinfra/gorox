// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Wrap revisers add something before or after response content.

package wrap

import (
	"errors"

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
	r.ConfigureInt8("rank", &r.rank, func(value int8) error {
		if value >= 6 && value < 26 {
			return nil
		}
		return errors.New(".rank has an invalid value")
	}, RankWrap)
}
func (r *wrapReviser) OnPrepare() {
	// TODO
}

func (r *wrapReviser) Rank() int8 { return r.rank }

func (r *wrapReviser) BeforeRecv(req Request, resp Response) { // sized
	// TODO
}
func (r *wrapReviser) OnRecv(req Request, resp Response, chain Chain) (Chain, bool) { // sized
	return chain, true
}

func (r *wrapReviser) BeforeSend(req Request, resp Response) { // sized
	// TODO
}
func (r *wrapReviser) OnSend(req Request, resp Response, content *Chain) { // sized
	if Debug() >= 2 {
		piece := GetPiece()
		piece.SetText([]byte("d"))
		content.PushTail(piece)
	}
}

func (r *wrapReviser) BeforeDraw(req Request, resp Response) { // unsized
	// TODO
}
func (r *wrapReviser) OnDraw(req Request, resp Response, chain Chain) (Chain, bool) { // unsized
	return chain, true
}
func (r *wrapReviser) FinishDraw(req Request, resp Response) { // unsized
	// TODO
}

func (r *wrapReviser) BeforeEcho(req Request, resp Response) { // unsized
	// TODO
	if Debug() >= 2 {
		Println("BeforeEcho")
	}
}
func (r *wrapReviser) OnEcho(req Request, resp Response, chunks *Chain) { // unsized
	if Debug() >= 2 {
		piece := GetPiece()
		piece.SetText([]byte("c"))
		chunks.PushTail(piece)
	}
}
func (r *wrapReviser) FinishEcho(req Request, resp Response) { // unsized
	// TODO
	if Debug() >= 2 {
		Println("FinishEcho")
	}
}
