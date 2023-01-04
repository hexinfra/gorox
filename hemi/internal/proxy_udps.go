// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// UDP/DTLS proxy dealet implementation.

package internal

func init() {
	RegisterUDPSDealet("udpsProxy", func(name string, stage *Stage, mesher *UDPSMesher) UDPSDealet {
		d := new(udpsProxy)
		d.onCreate(name, stage, mesher)
		return d
	})
}

// udpsProxy relays UDP/DTLS connections to another UDP/DTLS server.
type udpsProxy struct {
	// Mixins
	UDPSDealet_
	proxy_
	// Assocs
	mesher *UDPSMesher
	// States
}

func (d *udpsProxy) onCreate(name string, stage *Stage, mesher *UDPSMesher) {
	d.CompInit(name)
	d.proxy_.onCreate(stage)
	d.mesher = mesher
}

func (d *udpsProxy) OnConfigure() {
	d.proxy_.onConfigure(d)
}
func (d *udpsProxy) OnPrepare() {
	d.proxy_.onPrepare()
}

func (d *udpsProxy) OnShutdown() {
	d.mesher.SubDone()
}

func (d *udpsProxy) Deal(conn *UDPSConn) (next bool) {
	// TODO
	return
}
