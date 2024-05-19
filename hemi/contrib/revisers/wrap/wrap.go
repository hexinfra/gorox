// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Wrap revisers add something before or after response content.

package wrap

import (
	"errors"

	. "github.com/hexinfra/gorox/hemi"
	. "github.com/hexinfra/gorox/hemi/contrib/revisers"
)

func init() {
	RegisterReviser("wrapReviser", func(name string, stage *Stage, webapp *Webapp) Reviser {
		r := new(wrapReviser)
		r.onCreate(name, stage, webapp)
		return r
	})
}

// wrapReviser
type wrapReviser struct {
	// Parent
	Reviser_
	// Assocs
	stage  *Stage // current stage
	webapp *Webapp
	// States
	rank int8
}

func (r *wrapReviser) onCreate(name string, stage *Stage, webapp *Webapp) {
	r.MakeComp(name)
	r.stage = stage
	r.webapp = webapp
}
func (r *wrapReviser) OnShutdown() {
	r.webapp.DecSub()
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
func (r *wrapReviser) BeforeDraw(req Request, resp Response) { // vague
	// TODO
}
func (r *wrapReviser) OnInput(req Request, resp Response, chain *Chain) bool {
	// TODO
	return true
}
func (r *wrapReviser) FinishDraw(req Request, resp Response) { // vague
	// TODO
}

func (r *wrapReviser) BeforeSend(req Request, resp Response) { // sized
	// TODO
}
func (r *wrapReviser) BeforeEcho(req Request, resp Response) { // vague
	// TODO
	if DebugLevel() >= 2 {
		Println("BeforeEcho")
	}
}
func (r *wrapReviser) OnOutput(req Request, resp Response, chain *Chain) {
	if DebugLevel() >= 2 {
		piece := GetPiece()
		piece.SetText([]byte("d"))
		chain.PushTail(piece)
	}
}
func (r *wrapReviser) FinishEcho(req Request, resp Response) { // vague
	// TODO
	if DebugLevel() >= 2 {
		Println("FinishEcho")
	}
}
