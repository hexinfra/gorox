// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// TCP/TLS proxy runner implementation.

package internal

func init() {
	RegisterTCPSRunner("tcpsProxy", func(name string, stage *Stage, router *TCPSRouter) TCPSRunner {
		r := new(tcpsProxy)
		r.init(name, stage, router)
		return r
	})
}

// tcpsProxy passes TCP/TLS connections to another TCP/TLS server.
type tcpsProxy struct {
	// Mixins
	TCPSRunner_
	proxy_
	// Assocs
	router *TCPSRouter
	// States
}

func (r *tcpsProxy) init(name string, stage *Stage, router *TCPSRouter) {
	r.SetName(name)
	r.proxy_.init(stage)
	r.router = router
}

func (r *tcpsProxy) OnConfigure() {
	r.proxy_.onConfigure(r)
}
func (r *tcpsProxy) OnPrepare() {
	r.proxy_.onPrepare(r)
}
func (r *tcpsProxy) OnShutdown() {
	r.proxy_.onShutdown(r)
}

func (r *tcpsProxy) Process(conn *TCPSConn) (next bool) {
	return
}
