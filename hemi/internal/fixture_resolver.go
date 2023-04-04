// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// The name resolver. Resolveres DNS and names.

package internal

import (
	"time"
)

func init() {
	registerFixture(signResolver)
}

const signResolver = "resolver"

func createResolver(stage *Stage) *resolverFixture {
	resolver := new(resolverFixture)
	resolver.onCreate(stage)
	resolver.setShell(resolver)
	return resolver
}

// resolverFixture
type resolverFixture struct {
	// Mixins
	Component_
	// Assocs
	stage *Stage // current stage
	// States
}

func (f *resolverFixture) onCreate(stage *Stage) {
	f.MakeComp(signResolver)
	f.stage = stage
}
func (f *resolverFixture) OnShutdown() {
	close(f.Shut)
}

func (f *resolverFixture) OnConfigure() {
}
func (f *resolverFixture) OnPrepare() {
}

func (f *resolverFixture) run() { // goroutine
	Loop(time.Second, f.Shut, func(now time.Time) {
		// TODO
	})
	if IsDebug(2) {
		Debugln("resolver done")
	}
	f.stage.SubDone()
}

func (f *resolverFixture) Resolve(name string) (address string) {
	// TODO
	return ""
}
