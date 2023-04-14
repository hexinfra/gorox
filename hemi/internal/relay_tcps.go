// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// TCP/TLS relay filter implementation.

package internal

func init() {
	RegisterTCPSFilter("tcpsRelay", func(name string, stage *Stage, mesher *TCPSMesher) TCPSFilter {
		f := new(tcpsRelay)
		f.onCreate(name, stage, mesher)
		return f
	})
}

// tcpsRelay relays TCP/TLS connections to another TCP/TLS server.
type tcpsRelay struct {
	// Mixins
	TCPSFilter_
	proxy_
	// Assocs
	mesher *TCPSMesher
	// States
}

func (f *tcpsRelay) onCreate(name string, stage *Stage, mesher *TCPSMesher) {
	f.MakeComp(name)
	f.proxy_.onCreate(stage)
	f.mesher = mesher
}
func (f *tcpsRelay) OnShutdown() {
	f.mesher.SubDone()
}

func (f *tcpsRelay) OnConfigure() {
	f.proxy_.onConfigure(f)
}
func (f *tcpsRelay) OnPrepare() {
	f.proxy_.onPrepare(f)
}

func (f *tcpsRelay) Deal(conn *TCPSConn) (next bool) {
	// TODO
	return
}
