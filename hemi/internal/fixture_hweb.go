// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HWEB outgate.

package internal

import (
	"time"
)

func init() {
	registerFixture(signHWEBOutgate)
}

const signHWEBOutgate = "hwebOutgate"

func createHWEBOutgate(stage *Stage) *HWEBOutgate {
	hweb := new(HWEBOutgate)
	hweb.onCreate(stage)
	hweb.setShell(hweb)
	return hweb
}

// HWEBOutgate
type HWEBOutgate struct {
	// Mixins
	webOutgate_
	// States
}

func (f *HWEBOutgate) onCreate(stage *Stage) {
	f.webOutgate_.onCreate(signHWEBOutgate, stage)
}

func (f *HWEBOutgate) OnConfigure() {
	f.webOutgate_.onConfigure(f)
}
func (f *HWEBOutgate) OnPrepare() {
	f.webOutgate_.onPrepare(f)
}

func (f *HWEBOutgate) run() { // runner
	f.Loop(time.Second, func(now time.Time) {
		// TODO
	})
	if Debug() >= 2 {
		Println("hwebOutgate done")
	}
	f.stage.SubDone()
}
