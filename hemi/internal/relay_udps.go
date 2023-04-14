// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// UDP/DTLS relay filter implementation.

package internal

func init() {
	RegisterUDPSFilter("udpsRelay", func(name string, stage *Stage, mesher *UDPSMesher) UDPSFilter {
		f := new(udpsRelay)
		f.onCreate(name, stage, mesher)
		return f
	})
}

// udpsRelay relays UDP/DTLS connections to another UDP/DTLS server.
type udpsRelay struct {
	// Mixins
	UDPSFilter_
	proxy_
	// Assocs
	mesher *UDPSMesher
	// States
}

func (f *udpsRelay) onCreate(name string, stage *Stage, mesher *UDPSMesher) {
	f.MakeComp(name)
	f.proxy_.onCreate(stage)
	f.mesher = mesher
}
func (f *udpsRelay) OnShutdown() {
	f.mesher.SubDone()
}

func (f *udpsRelay) OnConfigure() {
	f.proxy_.onConfigure(f)
}
func (f *udpsRelay) OnPrepare() {
	f.proxy_.onPrepare(f)
}

func (f *udpsRelay) Deal(conn *UDPSConn) (next bool) {
	// TODO
	return
}
