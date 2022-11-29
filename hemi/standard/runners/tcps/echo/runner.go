// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Echo runners echo what client send.

package echo

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterTCPSRunner("echoRunner", func(name string, stage *Stage, mesher *TCPSMesher) TCPSRunner {
		r := new(echoRunner)
		r.onCreate(name, stage, mesher)
		return r
	})
}

// echoRunner
type echoRunner struct {
	// Mixins
	TCPSRunner_
	// Assocs
	stage  *Stage
	mesher *TCPSMesher
	// States
}

func (r *echoRunner) onCreate(name string, stage *Stage, mesher *TCPSMesher) {
	r.CompInit(name)
	r.stage = stage
	r.mesher = mesher
}

func (r *echoRunner) OnConfigure() {
}
func (r *echoRunner) OnPrepare() {
}

func (r *echoRunner) OnShutdown() {
	r.mesher.SubDone()
}

func (r *echoRunner) Process(conn *TCPSConn) (next bool) {
	p := make([]byte, 4096)
	for {
		n, err := conn.Read(p)
		if err != nil {
			break
		}
		conn.Write(p[:n])
	}
	return
}
