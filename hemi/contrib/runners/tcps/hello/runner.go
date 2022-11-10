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
	RegisterTCPSRunner("helloRunner", func(name string, stage *Stage, mesher *TCPSMesher) TCPSRunner {
		r := new(helloRunner)
		r.init(name, stage, mesher)
		return r
	})
}

// helloRunner
type helloRunner struct {
	// Mixins
	TCPSRunner_
	// Assocs
	stage  *Stage
	mesher *TCPSMesher
	// States
}

func (r *helloRunner) init(name string, stage *Stage, mesher *TCPSMesher) {
	r.SetName(name)
	r.stage = stage
	r.mesher = mesher
}

func (r *helloRunner) OnConfigure() {
}
func (r *helloRunner) OnPrepare() {
}
func (r *helloRunner) OnShutdown() {
}

func (r *helloRunner) Process(conn *TCPSConn) (next bool) {
	conn.Write([]byte("hello, world"))
	conn.Close()
	return
}
