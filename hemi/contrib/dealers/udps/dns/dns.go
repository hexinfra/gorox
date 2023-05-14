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
	RegisterUDPSDealer("dnsDealer", func(name string, stage *Stage, router *UDPSRouter) UDPSDealer {
		d := new(dnsDealer)
		d.onCreate(name, stage, router)
		return d
	})
}

// dnsDealer
type dnsDealer struct {
	// Mixins
	UDPSDealer_
	// Assocs
	stage  *Stage
	router *UDPSRouter
	// States
}

func (d *dnsDealer) onCreate(name string, stage *Stage, router *UDPSRouter) {
	d.MakeComp(name)
	d.stage = stage
	d.router = router
}
func (d *dnsDealer) OnShutdown() {
	d.router.SubDone()
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
