// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Access dealer allow limiting access to certain client addresses.

package access

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterTCPSDealer("accessDealer", func(name string, stage *Stage, mesher *TCPSMesher) TCPSDealer {
		d := new(accessDealer)
		d.onCreate(name, stage, mesher)
		return d
	})
}

// accessDealer
type accessDealer struct {
	// Mixins
	TCPSDealer_
	// Assocs
	stage  *Stage
	mesher *TCPSMesher
	// States
}

func (d *accessDealer) onCreate(name string, stage *Stage, mesher *TCPSMesher) {
	d.MakeComp(name)
	d.stage = stage
	d.mesher = mesher
}
func (d *accessDealer) OnShutdown() {
	d.mesher.SubDone()
}

func (d *accessDealer) OnConfigure() {
	// TODO
}
func (d *accessDealer) OnPrepare() {
	// TODO
}

func (d *accessDealer) Deal(conn *TCPSConn) (next bool) {
	// TODO
	return false
}
