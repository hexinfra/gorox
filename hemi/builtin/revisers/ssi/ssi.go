// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// SSI revisers can include other files in response content.

package ssi

import (
	"errors"

	. "github.com/hexinfra/gorox/hemi"
	. "github.com/hexinfra/gorox/hemi/builtin/revisers"
)

func init() {
	RegisterReviser("ssiReviser", func(compName string, stage *Stage, webapp *Webapp) Reviser {
		r := new(ssiReviser)
		r.onCreate(compName, stage, webapp)
		return r
	})
}

// ssiReviser
type ssiReviser struct {
	// Parent
	Reviser_
	// States
	rank int8
}

func (r *ssiReviser) onCreate(compName string, stage *Stage, webapp *Webapp) {
	r.Reviser_.OnCreate(compName, stage, webapp)
}
func (r *ssiReviser) OnShutdown() {
	r.Webapp().DecSub() // reviser
}

func (r *ssiReviser) OnConfigure() {
	// .rank
	r.ConfigureInt8("rank", &r.rank, func(value int8) error {
		if value >= 6 && value < 26 {
			return nil
		}
		return errors.New(".rank has an invalid value")
	}, RankSSI)
}
func (r *ssiReviser) OnPrepare() {
	// TODO
}

func (r *ssiReviser) Rank() int8 { return r.rank }

func (r *ssiReviser) BeforeRecv(req ServerRequest, resp ServerResponse) { // sized
	// TODO
}
func (r *ssiReviser) BeforeDraw(req ServerRequest, resp ServerResponse) { // vague
	// TODO
}
func (r *ssiReviser) OnInput(req ServerRequest, resp ServerResponse, input *Chain) bool {
	// TODO
	return true
}
func (r *ssiReviser) FinishDraw(req ServerRequest, resp ServerResponse) { // vague
	// TODO
}

func (r *ssiReviser) BeforeSend(req ServerRequest, resp ServerResponse) { // sized
	// TODO
}
func (r *ssiReviser) BeforeEcho(req ServerRequest, resp ServerResponse) { // vague
	// TODO
}
func (r *ssiReviser) OnOutput(req ServerRequest, resp ServerResponse, output *Chain) {
	// TODO
}
func (r *ssiReviser) FinishEcho(req ServerRequest, resp ServerResponse) { // vague
	// TODO
}
