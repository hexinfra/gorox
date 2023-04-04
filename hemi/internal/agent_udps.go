// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// UDP/DTLS agent dealet implementation.

package internal

func init() {
	RegisterUDPSDealet("udpsAgent", func(name string, stage *Stage, mesher *UDPSMesher) UDPSDealet {
		d := new(udpsAgent)
		d.onCreate(name, stage, mesher)
		return d
	})
}

// udpsAgent relays UDP/DTLS connections to another UDP/DTLS server.
type udpsAgent struct {
	// Mixins
	UDPSDealet_
	proxy_
	// Assocs
	mesher *UDPSMesher
	// States
}

func (d *udpsAgent) onCreate(name string, stage *Stage, mesher *UDPSMesher) {
	d.MakeComp(name)
	d.proxy_.onCreate(stage)
	d.mesher = mesher
}
func (d *udpsAgent) OnShutdown() {
	d.mesher.SubDone()
}

func (d *udpsAgent) OnConfigure() {
	d.proxy_.onConfigure(d)
}
func (d *udpsAgent) OnPrepare() {
	d.proxy_.onPrepare(d)
}

func (d *udpsAgent) Deal(conn *UDPSConn) (next bool) {
	// TODO
	return
}
