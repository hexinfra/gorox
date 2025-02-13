// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// DNS proxy.

package dns

import (
	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterUDPXDealet("dnsProxy", func(compName string, stage *Stage, router *UDPXRouter) UDPXDealet {
		d := new(dnsProxy)
		d.onCreate(compName, stage, router)
		return d
	})
}

// dnsProxy
type dnsProxy struct {
	// Parent
	UDPXDealet_
	// Assocs
	router *UDPXRouter
	// States
}

func (d *dnsProxy) onCreate(compName string, stage *Stage, router *UDPXRouter) {
	d.UDPXDealet_.OnCreate(compName, stage)
	d.router = router
}
func (d *dnsProxy) OnShutdown() {
	d.router.DecDealet()
}

func (d *dnsProxy) OnConfigure() {
	// TODO
}
func (d *dnsProxy) OnPrepare() {
	// TODO
}

func (d *dnsProxy) DealWith(conn *UDPXConn) (dealt bool) {
	// TODO
	return true
}
