// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// MongoDB proxy dealer passes conns to backend MongoDB servers.

package mongo

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterTCPSDealer("mongoProxy", func(name string, stage *Stage, mesher *TCPSMesher) TCPSDealer {
		d := new(mongoProxy)
		d.onCreate(name, stage, mesher)
		return d
	})
}

// mongoProxy
type mongoProxy struct {
	// Mixins
	TCPSDealer_
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

func (d *mongoProxy) Deal(conn *TCPSConn) (next bool) { // reverse only
	// TODO
	return false
}
