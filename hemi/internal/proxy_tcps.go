// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// TCP/TLS proxy dealet implementation.

package internal

func init() {
	RegisterTCPSDealet("tcpsProxy", func(name string, stage *Stage, mesher *TCPSMesher) TCPSDealet {
		d := new(tcpsProxy)
		d.onCreate(name, stage, mesher)
		return d
	})
}

// tcpsProxy relays TCP/TLS connections to another TCP/TLS server.
type tcpsProxy struct {
	// Mixins
	TCPSDealet_
	proxy_
	// Assocs
	mesher *TCPSMesher
	// States
}

func (d *tcpsProxy) onCreate(name string, stage *Stage, mesher *TCPSMesher) {
	d.CompInit(name)
	d.proxy_.onCreate(stage)
	d.mesher = mesher
}
func (d *tcpsProxy) OnShutdown() {
	d.mesher.SubDone()
}

func (d *tcpsProxy) OnConfigure() {
	d.proxy_.onConfigure(d)
}
func (d *tcpsProxy) OnPrepare() {
	d.proxy_.onPrepare()
}

func (d *tcpsProxy) Deal(conn *TCPSConn) (next bool) {
	// TODO
	return
}
