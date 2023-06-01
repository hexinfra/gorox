// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// DNS dealers can respond DNS requests.

package dns

import (
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterUDPSDealer("dnsDealer", func(name string, stage *Stage, mesher *UDPSMesher) UDPSDealer {
		d := new(dnsDealer)
		d.onCreate(name, stage, mesher)
		return d
	})
}

// dnsDealer
type dnsDealer struct {
	// Mixins
	UDPSDealer_
	// Assocs
	stage  *Stage
	mesher *UDPSMesher
	// States
}

func (d *dnsDealer) onCreate(name string, stage *Stage, mesher *UDPSMesher) {
	d.MakeComp(name)
	d.stage = stage
	d.mesher = mesher
}
func (d *dnsDealer) OnShutdown() {
	d.mesher.SubDone()
}

func (d *dnsDealer) OnConfigure() {
	// TODO
}
func (d *dnsDealer) OnPrepare() {
	// TODO
}

func (d *dnsDealer) Deal(link *UDPSLink) (next bool) {
	// TODO
	return
}
