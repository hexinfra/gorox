// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HRPC outgate.

package internal

import (
	"time"
)

func init() {
	registerFixture(signHRPCOutgate)
}

const signHRPCOutgate = "hrpcOutgate"

func createHRPCOutgate(stage *Stage) *HRPCOutgate {
	hrpc := new(HRPCOutgate)
	hrpc.onCreate(stage)
	hrpc.setShell(hrpc)
	return hrpc
}

// HRPCOutgate
type HRPCOutgate struct {
	// Mixins
	rpcOutgate_
	// States
}

func (f *HRPCOutgate) onCreate(stage *Stage) {
	f.rpcOutgate_.onCreate(signHRPCOutgate, stage)
}

func (f *HRPCOutgate) OnConfigure() {
	f.rpcOutgate_.onConfigure(f)
}
func (f *HRPCOutgate) OnPrepare() {
	f.rpcOutgate_.onPrepare(f)
}

func (f *HRPCOutgate) run() { // runner
	f.Loop(time.Second, func(now time.Time) {
		// TODO
	})
	if Debug() >= 2 {
		Println("hrpcOutgate done")
	}
	f.stage.SubDone()
}
