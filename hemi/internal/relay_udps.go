// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// UDP/DTLS relay runner implementation.

package internal

func init() {
	RegisterUDPSRunner("udpsRelay", func(name string, stage *Stage, router *UDPSRouter) UDPSRunner {
		r := new(udpsRelay)
		r.init(name, stage, router)
		return r
	})
}

// udpsRelay relays UDP/DTLS connections to another UDP/DTLS server.
type udpsRelay struct {
	// Mixins
	UDPSRunner_
	proxy_
	// Assocs
	router *UDPSRouter
	// States
}

func (r *udpsRelay) init(name string, stage *Stage, router *UDPSRouter) {
	r.SetName(name)
	r.proxy_.init(stage)
	r.router = router
}

func (r *udpsRelay) OnConfigure() {
	r.configure(r)
}
func (r *udpsRelay) OnPrepare() {
}
func (r *udpsRelay) OnShutdown() {
}

func (r *udpsRelay) Process(conn *UDPSConn) (next bool) {
	return
}
