// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// DNS dealets can respond DNS requests.

package dns

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterUDPSDealet("dnsDealet", func(name string, stage *Stage, mesher *UDPSMesher) UDPSDealet {
		d := new(dnsDealet)
		d.onCreate(name, stage, mesher)
		return d
	})
}

// dnsDealet
type dnsDealet struct {
	// Mixins
	UDPSDealet_
	// Assocs
	stage  *Stage
	mesher *UDPSMesher
	// States
}

func (d *dnsDealet) onCreate(name string, stage *Stage, mesher *UDPSMesher) {
	d.MakeComp(name)
	d.stage = stage
	d.mesher = mesher
}
func (d *dnsDealet) OnShutdown() {
	d.mesher.SubDone()
}

func (d *dnsDealet) OnConfigure() {
	// TODO
}
func (d *dnsDealet) OnPrepare() {
	// TODO
}

func (d *dnsDealet) Deal(conn *UDPSConn) (dealt bool) {
	// TODO
	return true
}
