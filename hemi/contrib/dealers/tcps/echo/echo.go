// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Echo dealers echo what client send.

package echo

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterTCPSDealer("echoDealer", func(name string, stage *Stage, mesher *TCPSMesher) TCPSDealer {
		d := new(echoDealer)
		d.onCreate(name, stage, mesher)
		return d
	})
}

// echoDealer
type echoDealer struct {
	// Mixins
	TCPSDealer_
	// Assocs
	stage  *Stage
	mesher *TCPSMesher
	// States
}

func (d *echoDealer) onCreate(name string, stage *Stage, mesher *TCPSMesher) {
	d.MakeComp(name)
	d.stage = stage
	d.mesher = mesher
}
func (d *echoDealer) OnShutdown() {
	d.mesher.SubDone()
}

func (d *echoDealer) OnConfigure() {
	// TODO
}
func (d *echoDealer) OnPrepare() {
	// TODO
}

func (d *echoDealer) Deal(conn *TCPSConn) (next bool) {
	// TODO
	return false
}