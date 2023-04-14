// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// UDP/DTLS proxy filter implementation.

package internal

func init() {
	RegisterUDPSFilter("udpsProxy", func(name string, stage *Stage, mesher *UDPSMesher) UDPSFilter {
		f := new(udpsProxy)
		f.onCreate(name, stage, mesher)
		return f
	})
}

// udpsProxy relays UDP/DTLS connections to another UDP/DTLS server.
type udpsProxy struct {
	// Mixins
	UDPSFilter_
	proxy_
	// Assocs
	mesher *UDPSMesher
	// States
}

func (f *udpsProxy) onCreate(name string, stage *Stage, mesher *UDPSMesher) {
	f.MakeComp(name)
	f.proxy_.onCreate(stage)
	f.mesher = mesher
}
func (f *udpsProxy) OnShutdown() {
	f.mesher.SubDone()
}

func (f *udpsProxy) OnConfigure() {
	f.proxy_.onConfigure(f)
}
func (f *udpsProxy) OnPrepare() {
	f.proxy_.onPrepare(f)
}

func (f *udpsProxy) Deal(conn *UDPSConn) (next bool) {
	// TODO
	return
}
