// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// A demo optware.

package demo

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterOptware("demo", func(name string, stage *Stage) Optware {
		o := new(demoOptware)
		o.init(name, stage)
		return o
	})
}

type demoOptware struct {
	// Mixins
	Optware_
	// Assocs
	stage *Stage
	// States
}

func (o *demoOptware) init(name string, stage *Stage) {
	o.SetName(name)
	o.stage = stage
}

func (o *demoOptware) OnConfigure() {
}
func (o *demoOptware) OnPrepare() {
}
func (o *demoOptware) OnShutdown() {
}

func (o *demoOptware) Run() {
	// Do nothing.
}