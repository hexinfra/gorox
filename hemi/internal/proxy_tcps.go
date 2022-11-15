// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// TCP/TLS proxy runner implementation.

package internal

func init() {
	RegisterTCPSRunner("tcpsProxy", func(name string, stage *Stage, mesher *TCPSMesher) TCPSRunner {
		r := new(tcpsProxy)
		r.init(name, stage, mesher)
		return r
	})
}

// tcpsProxy passes TCP/TLS connections to another TCP/TLS server.
type tcpsProxy struct {
	// Mixins
	TCPSRunner_
	proxy_
	// Assocs
	mesher *TCPSMesher
	// States
}

func (r *tcpsProxy) init(name string, stage *Stage, mesher *TCPSMesher) {
	r.InitComp(name)
	r.proxy_.init(stage)
	r.mesher = mesher
}

func (r *tcpsProxy) OnConfigure() {
	r.proxy_.onConfigure(r)
}
func (r *tcpsProxy) OnPrepare() {
	r.proxy_.onPrepare(r)
}
func (r *tcpsProxy) OnShutdown() {
	r.proxy_.onShutdown(r)
	r.mesher.SubDone()
}

func (r *tcpsProxy) Process(conn *TCPSConn) (next bool) {
	// TODO
	return
}
