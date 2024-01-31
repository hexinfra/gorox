// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Access dealet allow limiting access to certain client addresses.

package access

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterTCPSDealet("accessDealet", func(name string, stage *Stage, mesher *TCPSMesher) TCPSDealet {
		d := new(accessDealet)
		d.onCreate(name, stage, mesher)
		return d
	})
}

// accessDealet
type accessDealet struct {
	// Mixins
	TCPSDealet_
	// Assocs
	stage  *Stage
	mesher *TCPSMesher
	// States
}

func (d *accessDealet) onCreate(name string, stage *Stage, mesher *TCPSMesher) {
	d.MakeComp(name)
	d.stage = stage
	d.mesher = mesher
}
func (d *accessDealet) OnShutdown() {
	d.mesher.SubDone()
}

func (d *accessDealet) OnConfigure() {
	// TODO
}
func (d *accessDealet) OnPrepare() {
	// TODO
}

func (d *accessDealet) Deal(conn *TCPSConn) (dealt bool) {
	return true
}
