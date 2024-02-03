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
	RegisterUDPSDealet("dnsDealet", func(name string, stage *Stage, router *UDPSRouter) UDPSDealet {
		d := new(dnsDealet)
		d.onCreate(name, stage, router)
		return d
	})
}

// dnsDealet
type dnsDealet struct {
	// Mixins
	UDPSDealet_
	// Assocs
	stage  *Stage
	router *UDPSRouter
	// States
}

func (d *dnsDealet) onCreate(name string, stage *Stage, router *UDPSRouter) {
	d.MakeComp(name)
	d.stage = stage
	d.router = router
}
func (d *dnsDealet) OnShutdown() {
	d.router.SubDone()
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
