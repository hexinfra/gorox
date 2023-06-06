// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// The name resolver. Resolves DNS and names.

package internal

import (
	"time"
)

func init() {
	registerFixture(signResolv)
}

const signResolv = "resolv"

func createResolv(stage *Stage) *resolvFixture {
	resolv := new(resolvFixture)
	resolv.onCreate(stage)
	resolv.setShell(resolv)
	return resolv
}

// resolvFixture
type resolvFixture struct {
	// Mixins
	fixture_
	// States
}

func (f *resolvFixture) onCreate(stage *Stage) {
	f.fixture_.onCreate(signResolv, stage)
}
func (f *resolvFixture) OnShutdown() {
	close(f.Shut)
}

func (f *resolvFixture) OnConfigure() {
}
func (f *resolvFixture) OnPrepare() {
}

func (f *resolvFixture) run() { // goroutine
	f.Loop(time.Second, func(now time.Time) {
		// TODO
	})
	if Debug() >= 2 {
		Println("resolv done")
	}
	f.stage.SubDone()
}

func (f *resolvFixture) Register(name string, address string) bool {
	// TODO
	return false
}

func (f *resolvFixture) Resolve(name string) (address string) {
	// TODO
	return ""
}
