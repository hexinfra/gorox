// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Hello dealets print a welcome text.

package hello

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterTCPSDealet("helloDealet", func(name string, stage *Stage, mesher *TCPSMesher) TCPSDealet {
		d := new(helloDealet)
		d.onCreate(name, stage, mesher)
		return d
	})
}

// helloDealet
type helloDealet struct {
	// Mixins
	TCPSDealet_
	// Assocs
	stage  *Stage
	mesher *TCPSMesher
	// States
}

func (d *helloDealet) onCreate(name string, stage *Stage, mesher *TCPSMesher) {
	d.SetUp(name)
	d.stage = stage
	d.mesher = mesher
}
func (d *helloDealet) OnShutdown() {
	d.mesher.SubDone()
}

func (d *helloDealet) OnConfigure() {
}
func (d *helloDealet) OnPrepare() {
}

func (d *helloDealet) Deal(conn *TCPSConn) (next bool) {
	conn.Write([]byte("hello, world"))
	conn.Close()
	return false
}
