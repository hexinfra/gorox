// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Echo dealets echo what client send.

package echo

import (
	. "github.com/hexinfra/gorox/hemi/internal"
	"io"
)

func init() {
	RegisterTCPSDealet("echoDealet", func(name string, stage *Stage, mesher *TCPSMesher) TCPSDealet {
		d := new(echoDealet)
		d.onCreate(name, stage, mesher)
		return d
	})
}

// echoDealet
type echoDealet struct {
	// Mixins
	TCPSDealet_
	// Assocs
	stage  *Stage
	mesher *TCPSMesher
	// States
}

func (d *echoDealet) onCreate(name string, stage *Stage, mesher *TCPSMesher) {
	d.SetUp(name)
	d.stage = stage
	d.mesher = mesher
}
func (d *echoDealet) OnShutdown() {
	d.mesher.SubDone()
}

func (d *echoDealet) OnConfigure() {
}
func (d *echoDealet) OnPrepare() {
}

func (d *echoDealet) Deal(conn *TCPSConn) (next bool) {
	// TODO: deadline?
	io.Copy(conn, conn)
	return false
}
