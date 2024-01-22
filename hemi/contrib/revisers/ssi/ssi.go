// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// SSI revisers can include other files in response content.

package ssi

import (
	"errors"

	. "github.com/hexinfra/gorox/hemi/contrib/revisers"
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterReviser("ssiReviser", func(name string, stage *Stage, webapp *Webapp) Reviser {
		r := new(ssiReviser)
		r.onCreate(name, stage, webapp)
		return r
	})
}

// ssiReviser
type ssiReviser struct {
	// Mixins
	Reviser_
	// Assocs
	stage  *Stage
	webapp *Webapp
	// States
	rank int8
}

func (r *ssiReviser) onCreate(name string, stage *Stage, webapp *Webapp) {
	r.MakeComp(name)
	r.stage = stage
	r.webapp = webapp
}
func (r *ssiReviser) OnShutdown() {
	r.webapp.SubDone()
}

func (r *ssiReviser) OnConfigure() {
	// rank
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

func (r *ssiReviser) BeforeRecv(req Request, resp Response) { // sized
	// TODO
}
func (r *ssiReviser) BeforeDraw(req Request, resp Response) { // vague
	// TODO
}
func (r *ssiReviser) OnInput(req Request, resp Response, chain *Chain) bool { // sized
	return true
}
func (r *ssiReviser) FinishDraw(req Request, resp Response) { // vague
	// TODO
}

func (r *ssiReviser) BeforeSend(req Request, resp Response) { // sized
	// TODO
}
func (r *ssiReviser) BeforeEcho(req Request, resp Response) { // vague
	// TODO
}
func (r *ssiReviser) OnOutput(req Request, resp Response, chain *Chain) { // sized
}
func (r *ssiReviser) FinishEcho(req Request, resp Response) { // vague
	// TODO
}
