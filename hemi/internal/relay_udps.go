// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// UDP/DTLS relay implementation.

package internal

func init() {
	RegisterUDPSDealer("udpsRelay", func(name string, stage *Stage, router *UDPSRouter) UDPSDealer {
		d := new(udpsRelay)
		d.onCreate(name, stage, router)
		return d
	})
}

// udpsRelay passes UDP/DTLS connections to backend UDP/DTLS server.
type udpsRelay struct {
	// Mixins
	UDPSDealer_
	// Assocs
	stage   *Stage
	router  *UDPSRouter
	backend *UDPSBackend
	// States
}

func (d *udpsRelay) onCreate(name string, stage *Stage, router *UDPSRouter) {
	d.MakeComp(name)
	d.stage = stage
	d.router = router
}
func (d *udpsRelay) OnShutdown() {
	d.router.SubDone()
}

func (d *udpsRelay) OnConfigure() {
	// toBackend
	if v, ok := d.Find("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := d.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else if udpsBackend, ok := backend.(*UDPSBackend); ok {
				d.backend = udpsBackend
			} else {
				UseExitf("incorrect backend '%s' for udpsRelay\n", name)
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else {
		UseExitln("toBackend is required for udpsRelay")
	}
}
func (d *udpsRelay) OnPrepare() {
}

func (d *udpsRelay) Deal(conn *UDPSConn) (next bool) { // reverse only
	// TODO
	return
}
