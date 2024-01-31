// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// MongoDB proxy dealet passes conns to backend MongoDB servers.

package mongo

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterTCPSDealet("mongoProxy", func(name string, stage *Stage, mesher *TCPSMesher) TCPSDealet {
		d := new(mongoProxy)
		d.onCreate(name, stage, mesher)
		return d
	})
}

// mongoProxy
type mongoProxy struct {
	// Mixins
	TCPSDealet_
	// Assocs
	stage  *Stage
	mesher *TCPSMesher
	// States
}

func (d *mongoProxy) onCreate(name string, stage *Stage, mesher *TCPSMesher) {
	d.MakeComp(name)
	d.stage = stage
	d.mesher = mesher
}
func (d *mongoProxy) OnShutdown() {
	d.mesher.SubDone()
}

func (d *mongoProxy) OnConfigure() {
	// TODO
}
func (d *mongoProxy) OnPrepare() {
	// TODO
}

func (d *mongoProxy) OnSetup(conn *TCPSConn) (next bool) {
	return
}
func (d *mongoProxy) OnInput(buf *Buffer, end bool) (next bool) {
	return
}
func (d *mongoProxy) OnOutput(buf *Buffer, end bool) (next bool) {
	return
}
