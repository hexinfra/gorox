// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Echo dealets echo what client send.

package echo

import (
	. "github.com/hexinfra/gorox/hemi/internal"
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
	d.MakeComp(name)
	d.stage = stage
	d.mesher = mesher
}
func (d *echoDealet) OnShutdown() {
	d.mesher.SubDone()
}

func (d *echoDealet) OnConfigure() {
	// TODO
}
func (d *echoDealet) OnPrepare() {
	// TODO
}

func (d *echoDealet) OnSetup(conn *TCPSConn) (next bool) {
	return
}
func (d *echoDealet) OnInput(buf *Buffer, end bool) (next bool) {
	/*
		conn := buf.Conn()
		conn.Write(buf.Data())
		if end {
			conn.CloseWrite()
		}
	*/
	return
}
func (d *echoDealet) OnOutput(buf *Buffer, end bool) (next bool) {
	return
}
