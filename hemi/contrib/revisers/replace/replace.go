// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Replace revisers replace something in response content.

package replace

import (
	"errors"

	. "github.com/hexinfra/gorox/hemi/contrib/revisers"
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterReviser("replaceReviser", func(name string, stage *Stage, webapp *Webapp) Reviser {
		r := new(replaceReviser)
		r.onCreate(name, stage, webapp)
		return r
	})
}

// replaceReviser
type replaceReviser struct {
	// Mixins
	Reviser_
	// Assocs
	stage  *Stage
	webapp *Webapp
	// States
	rank int8
}

func (r *replaceReviser) onCreate(name string, stage *Stage, webapp *Webapp) {
	r.MakeComp(name)
	r.stage = stage
	r.webapp = webapp
}
func (r *replaceReviser) OnShutdown() {
	r.webapp.SubDone()
}

func (r *replaceReviser) OnConfigure() {
	// rank
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
func (r *replaceReviser) OnRecv(req Request, resp Response, chain Chain) (Chain, bool) { // sized
	return chain, true
}

func (r *replaceReviser) BeforeSend(req Request, resp Response) { // sized
	// TODO
}
func (r *replaceReviser) OnSend(req Request, resp Response, content *Chain) { // sized
}

func (r *replaceReviser) BeforeDraw(req Request, resp Response) { // unsized
	// TODO
}
func (r *replaceReviser) OnDraw(req Request, resp Response, chain Chain) (Chain, bool) { // unsized
	return chain, true
}
func (r *replaceReviser) FinishDraw(req Request, resp Response) { // unsized
	// TODO
}

func (r *replaceReviser) BeforeEcho(req Request, resp Response) { // unsized
	// TODO
}
func (r *replaceReviser) OnEcho(req Request, resp Response, chunks *Chain) { // unsized
}
func (r *replaceReviser) FinishEcho(req Request, resp Response) { // unsized
	// TODO
}
