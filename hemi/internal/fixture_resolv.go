// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
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
	Component_
	// Assocs
	stage *Stage // current stage
	// States
}

func (f *resolvFixture) onCreate(stage *Stage) {
	f.CompInit(signResolv)
	f.stage = stage
}
func (f *resolvFixture) OnShutdown() {
	f.Shutdown()
}

func (f *resolvFixture) OnConfigure() {
}
func (f *resolvFixture) OnPrepare() {
}

func (f *resolvFixture) run() { // goroutine
	Loop(time.Second, f.Shut, func(now time.Time) {
		// TODO
	})
	if IsDebug(2) {
		Debugln("resolve done")
	}
	f.stage.SubDone()
}

func (f *resolvFixture) Resolve(name string) (address string) {
	// TODO
	return ""
}
