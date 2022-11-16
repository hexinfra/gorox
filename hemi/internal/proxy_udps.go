// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// UDP/DTLS proxy runner implementation.

package internal

func init() {
	RegisterUDPSRunner("udpsProxy", func(name string, stage *Stage, mesher *UDPSMesher) UDPSRunner {
		r := new(udpsProxy)
		r.init(name, stage, mesher)
		return r
	})
}

// udpsProxy passes UDP/DTLS connections to another UDP/DTLS server.
type udpsProxy struct {
	// Mixins
	UDPSRunner_
	proxy_
	// Assocs
	mesher *UDPSMesher
	// States
}

func (r *udpsProxy) init(name string, stage *Stage, mesher *UDPSMesher) {
	r.CompInit(name)
	r.proxy_.init(stage)
	r.mesher = mesher
}

func (r *udpsProxy) OnConfigure() {
	r.proxy_.onConfigure(r)
}
func (r *udpsProxy) OnPrepare() {
	r.proxy_.onPrepare()
}

func (r *udpsProxy) OnShutdown() {
	r.mesher.SubDone()
}

func (r *udpsProxy) Process(conn *UDPSConn) (next bool) {
	// TODO
	return
}
