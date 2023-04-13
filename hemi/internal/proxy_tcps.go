// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// TCP/TLS proxy filter implementation.

package internal

func init() {
	RegisterTCPSFilter("tcpsProxy", func(name string, stage *Stage, mesher *TCPSMesher) TCPSFilter {
		f := new(tcpsProxy)
		f.onCreate(name, stage, mesher)
		return f
	})
}

// tcpsProxy relays TCP/TLS connections to another TCP/TLS server.
type tcpsProxy struct {
	// Mixins
	TCPSFilter_
	proxy_
	// Assocs
	mesher *TCPSMesher
	// States
}

func (f *tcpsProxy) onCreate(name string, stage *Stage, mesher *TCPSMesher) {
	f.MakeComp(name)
	f.proxy_.onCreate(stage)
	f.mesher = mesher
}
func (f *tcpsProxy) OnShutdown() {
	f.mesher.SubDone()
}

func (f *tcpsProxy) OnConfigure() {
	f.proxy_.onConfigure(f)
}
func (f *tcpsProxy) OnPrepare() {
	f.proxy_.onPrepare(f)
}

func (f *tcpsProxy) Deal(conn *TCPSConn) (next bool) {
	// TODO
	// NOTE: if configured as forward proxy, work as a SOCKS server? HTTP tunnel?
	return
}
