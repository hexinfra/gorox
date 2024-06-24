// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// DNS proxy.

package dns

import (
	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterUDPXDealet("dnsProxy", func(name string, stage *Stage, router *UDPXRouter) UDPXDealet {
		d := new(dnsProxy)
		d.onCreate(name, stage, router)
		return d
	})
}

// dnsProxy
type dnsProxy struct {
	// Parent
	UDPXDealet_
	// Assocs
	stage  *Stage // current stage
	router *UDPXRouter
	// States
}

func (d *dnsProxy) onCreate(name string, stage *Stage, router *UDPXRouter) {
	d.MakeComp(name)
	d.stage = stage
	d.router = router
}
func (d *dnsProxy) OnShutdown() {
	d.router.DecSub() // dealet
}

func (d *dnsProxy) OnConfigure() {
	// TODO
}
func (d *dnsProxy) OnPrepare() {
	// TODO
}

func (d *dnsProxy) Deal(conn *UDPXConn) (dealt bool) {
	// TODO
	return true
}
