// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Hello runners print a welcome text.

package hello

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterTCPSRunner("helloRunner", func(name string, stage *Stage, router *TCPSRouter) TCPSRunner {
		r := new(helloRunner)
		r.init(name, stage, router)
		return r
	})
}

// helloRunner
type helloRunner struct {
	// Mixins
	TCPSRunner_
	// Assocs
	stage  *Stage
	router *TCPSRouter
	// States
}

func (r *helloRunner) init(name string, stage *Stage, router *TCPSRouter) {
	r.SetName(name)
	r.stage = stage
	r.router = router
}

func (r *helloRunner) OnConfigure() {
}
func (r *helloRunner) OnPrepare() {
}
func (r *helloRunner) OnShutdown() {
}

func (r *helloRunner) Process(conn *TCPSConn) (next bool) {
	conn.Write([]byte("hello, world"))
	return
}
