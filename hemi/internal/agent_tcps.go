// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// TCP/TLS agent dealet implementation.

package internal

func init() {
	RegisterTCPSDealet("tcpsAgent", func(name string, stage *Stage, mesher *TCPSMesher) TCPSDealet {
		d := new(tcpsAgent)
		d.onCreate(name, stage, mesher)
		return d
	})
}

// tcpsAgent relays TCP/TLS connections to another TCP/TLS server.
type tcpsAgent struct {
	// Mixins
	TCPSDealet_
	proxy_
	// Assocs
	mesher *TCPSMesher
	// States
}

func (d *tcpsAgent) onCreate(name string, stage *Stage, mesher *TCPSMesher) {
	d.MakeComp(name)
	d.proxy_.onCreate(stage)
	d.mesher = mesher
}
func (d *tcpsAgent) OnShutdown() {
	d.mesher.SubDone()
}

func (d *tcpsAgent) OnConfigure() {
	d.proxy_.onConfigure(d)
}
func (d *tcpsAgent) OnPrepare() {
	d.proxy_.onPrepare(d)
}

func (d *tcpsAgent) Deal(conn *TCPSConn) (next bool) {
	// TODO
	return
}
