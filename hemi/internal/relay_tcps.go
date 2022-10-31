// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// TCP/TLS relay runner implementation.

package internal

func init() {
	RegisterTCPSRunner("tcpsRelay", func(name string, stage *Stage, router *TCPSRouter) TCPSRunner {
		r := new(tcpsRelay)
		r.init(name, stage, router)
		return r
	})
}

// tcpsRelay relays TCP/TLS connections to another TCP/TLS server.
type tcpsRelay struct {
	// Mixins
	TCPSRunner_
	proxy_
	// Assocs
	router *TCPSRouter
	// States
}

func (r *tcpsRelay) init(name string, stage *Stage, router *TCPSRouter) {
	r.SetName(name)
	r.proxy_.init(stage)
	r.router = router
}

func (r *tcpsRelay) OnConfigure() {
	r.proxy_.onConfigure(r)
}
func (r *tcpsRelay) OnPrepare() {
	r.proxy_.onPrepare(r)
}
func (r *tcpsRelay) OnShutdown() {
	r.proxy_.onShutdown(r)
}

func (r *tcpsRelay) Process(conn *TCPSConn) (next bool) {
	return
}
