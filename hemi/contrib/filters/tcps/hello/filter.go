// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Hello filters print a welcome text.

package hello

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterTCPSFilter("helloFilter", func(name string, stage *Stage, router *TCPSRouter) TCPSFilter {
		f := new(helloFilter)
		f.init(name, stage, router)
		return f
	})
}

// helloFilter
type helloFilter struct {
	// Mixins
	TCPSFilter_
	// Assocs
	stage  *Stage
	router *TCPSRouter
	// States
}

func (f *helloFilter) init(name string, stage *Stage, router *TCPSRouter) {
	f.SetName(name)
	f.stage = stage
	f.router = router
}

func (f *helloFilter) OnConfigure() {
}
func (f *helloFilter) OnPrepare() {
}
func (f *helloFilter) OnShutdown() {
}

func (f *helloFilter) Process(conn *TCPSConn) (next bool) {
	conn.Write([]byte("hello, world"))
	return
}
