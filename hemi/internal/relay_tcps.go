// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// TCP/TLS relay filter implementation.

package internal

func init() {
	RegisterTCPSFilter("tcpsRelay", func(name string, stage *Stage, router *TCPSRouter) TCPSFilter {
		f := new(tcpsRelay)
		f.init(name, stage, router)
		return f
	})
}

// tcpsRelay relays TCP/TLS connections to another TCP/TLS server.
type tcpsRelay struct {
	// Mixins
	TCPSFilter_
	proxy_
	// Assocs
	router *TCPSRouter
	// States
}

func (f *tcpsRelay) init(name string, stage *Stage, router *TCPSRouter) {
	f.SetName(name)
	f.proxy_.init(stage)
	f.router = router
}

func (f *tcpsRelay) OnConfigure() {
	f.proxy_.onConfigure(f)
}
func (f *tcpsRelay) OnPrepare() {
	f.proxy_.onPrepare(f)
}
func (f *tcpsRelay) OnShutdown() {
	f.proxy_.onShutdown(f)
}

func (f *tcpsRelay) Process(conn *TCPSConn) (next bool) {
	return
}
