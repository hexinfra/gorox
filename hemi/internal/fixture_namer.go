// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// The name resolver. Resolves DNS and names.

package internal

import (
	"time"
)

func init() {
	registerFixture(signNamer)
}

const signNamer = "namer"

func createNamer(stage *Stage) *namerFixture {
	namer := new(namerFixture)
	namer.onCreate(stage)
	namer.setShell(namer)
	return namer
}

// namerFixture
type namerFixture struct {
	// Mixins
	fixture_
	// States
}

func (f *namerFixture) onCreate(stage *Stage) {
	f.fixture_.onCreate(signNamer, stage)
}
func (f *namerFixture) OnShutdown() {
	close(f.ShutChan) // notifies run()
}

func (f *namerFixture) OnConfigure() {
}
func (f *namerFixture) OnPrepare() {
}

func (f *namerFixture) run() { // runner
	f.Loop(time.Second, func(now time.Time) {
		// TODO
	})
	if Debug() >= 2 {
		Println("namer done")
	}
	f.stage.SubDone()
}

func (f *namerFixture) Register(name string, address string) bool {
	// TODO
	return false
}

func (f *namerFixture) Resolve(name string) (address string) {
	// TODO
	return ""
}
