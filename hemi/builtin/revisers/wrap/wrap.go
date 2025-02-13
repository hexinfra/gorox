// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Wrap revisers add something before or after response content.

package wrap

import (
	"errors"

	. "github.com/hexinfra/gorox/hemi"
	. "github.com/hexinfra/gorox/hemi/builtin/revisers"
)

func init() {
	RegisterReviser("wrapReviser", func(compName string, stage *Stage, webapp *Webapp) Reviser {
		r := new(wrapReviser)
		r.onCreate(compName, stage, webapp)
		return r
	})
}

// wrapReviser
type wrapReviser struct {
	// Parent
	Reviser_
	// States
	rank int8
}

func (r *wrapReviser) onCreate(compName string, stage *Stage, webapp *Webapp) {
	r.Reviser_.OnCreate(compName, stage, webapp)
}
func (r *wrapReviser) OnShutdown() { r.Webapp().DecReviser() }

func (r *wrapReviser) OnConfigure() {
	// .rank
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

func (r *wrapReviser) BeforeRecv(req ServerRequest, resp ServerResponse) { // sized
	// TODO
}
func (r *wrapReviser) BeforeDraw(req ServerRequest, resp ServerResponse) { // vague
	// TODO
}
func (r *wrapReviser) OnInput(req ServerRequest, resp ServerResponse, input *Chain) bool {
	// TODO
	return true
}
func (r *wrapReviser) FinishDraw(req ServerRequest, resp ServerResponse) { // vague
	// TODO
}

func (r *wrapReviser) BeforeSend(req ServerRequest, resp ServerResponse) { // sized
	// TODO
}
func (r *wrapReviser) BeforeEcho(req ServerRequest, resp ServerResponse) { // vague
	// TODO
	if DebugLevel() >= 2 {
		Println("BeforeEcho")
	}
}
func (r *wrapReviser) OnOutput(req ServerRequest, resp ServerResponse, output *Chain) {
	if DebugLevel() >= 2 {
		piece := GetPiece()
		piece.SetText([]byte("d"))
		output.PushTail(piece)
	}
}
func (r *wrapReviser) FinishEcho(req ServerRequest, resp ServerResponse) { // vague
	// TODO
	if DebugLevel() >= 2 {
		Println("FinishEcho")
	}
}
