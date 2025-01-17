// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Replace revisers replace something in response content.

package replace

import (
	"errors"

	. "github.com/hexinfra/gorox/hemi"
	. "github.com/hexinfra/gorox/hemi/classic/revisers"
)

func init() {
	RegisterReviser("replaceReviser", func(compName string, stage *Stage, webapp *Webapp) Reviser {
		r := new(replaceReviser)
		r.onCreate(compName, stage, webapp)
		return r
	})
}

// replaceReviser
type replaceReviser struct {
	// Parent
	Reviser_
	// States
	rank int8
}

func (r *replaceReviser) onCreate(compName string, stage *Stage, webapp *Webapp) {
	r.Reviser_.OnCreate(compName, stage, webapp)
}
func (r *replaceReviser) OnShutdown() {
	r.Webapp().DecSub() // reviser
}

func (r *replaceReviser) OnConfigure() {
	// .rank
	r.ConfigureInt8("rank", &r.rank, func(value int8) error {
		if value >= 6 && value < 26 {
			return nil
		}
		return errors.New(".rank has an invalid value")
	}, RankReplace)
}
func (r *replaceReviser) OnPrepare() {
	// TODO
}

func (r *replaceReviser) Rank() int8 { return r.rank }

func (r *replaceReviser) BeforeRecv(req Request, resp Response) { // sized
	// TODO
}
func (r *replaceReviser) BeforeDraw(req Request, resp Response) { // vague
	// TODO
}
func (r *replaceReviser) OnInput(req Request, resp Response, input *Chain) bool {
	// TODO
	return true
}
func (r *replaceReviser) FinishDraw(req Request, resp Response) { // vague
	// TODO
}

func (r *replaceReviser) BeforeSend(req Request, resp Response) { // sized
	// TODO
}
func (r *replaceReviser) BeforeEcho(req Request, resp Response) { // vague
	// TODO
}
func (r *replaceReviser) OnOutput(req Request, resp Response, output *Chain) {
	// TODO
}
func (r *replaceReviser) FinishEcho(req Request, resp Response) { // vague
	// TODO
}
