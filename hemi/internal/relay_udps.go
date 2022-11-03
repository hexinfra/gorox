// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// UDP/DTLS relay filter implementation.

package internal

func init() {
	RegisterUDPSFilter("udpsRelay", func(name string, stage *Stage, router *UDPSRouter) UDPSFilter {
		f := new(udpsRelay)
		f.init(name, stage, router)
		return f
	})
}

// udpsRelay relays UDP/DTLS connections to another UDP/DTLS server.
type udpsRelay struct {
	// Mixins
	UDPSFilter_
	proxy_
	// Assocs
	router *UDPSRouter
	// States
}

func (f *udpsRelay) init(name string, stage *Stage, router *UDPSRouter) {
	f.SetName(name)
	f.proxy_.init(stage)
	f.router = router
}

func (f *udpsRelay) OnConfigure() {
	f.proxy_.onConfigure(f)
}
func (f *udpsRelay) OnPrepare() {
	f.proxy_.onPrepare(f)
}
func (f *udpsRelay) OnShutdown() {
	f.proxy_.onShutdown(f)
}

func (f *udpsRelay) Process(conn *UDPSConn) (next bool) {
	return
}
