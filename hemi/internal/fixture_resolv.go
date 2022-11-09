// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// The name resolver. Resolves DNS and names.

package internal

import (
	"fmt"
	"time"
)

func init() {
	registerFixture(signResolv)
}

const signResolv = "resolv"

func createResolv(stage *Stage) *resolvFixture {
	resolv := new(resolvFixture)
	resolv.init(stage)
	resolv.setShell(resolv)
	return resolv
}

// resolvFixture
type resolvFixture struct {
	// Mixins
	fixture_
	// States
}

func (f *resolvFixture) init(stage *Stage) {
	f.fixture_.init(signResolv, stage)
}

func (f *resolvFixture) OnConfigure() {
}
func (f *resolvFixture) OnPrepare() {
}
func (f *resolvFixture) OnShutdown() {
	f.SetShut()
}

func (f *resolvFixture) run() { // goroutine
	for !f.IsShut() {
		time.Sleep(time.Second)
	}
	if Debug(2) {
		fmt.Println("resolve done")
	}
	f.stage.SubDone()
}

func (f *resolvFixture) Resolve(name string) (address string) {
	// TODO
	return ""
}
