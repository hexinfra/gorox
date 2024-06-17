// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// UDPX (UDP/TLS/UDS) reverse proxy.

package hemi

func init() {
	RegisterUDPXDealet("udpxProxy", func(name string, stage *Stage, router *UDPXRouter) UDPXDealet {
		d := new(udpxProxy)
		d.onCreate(name, stage, router)
		return d
	})
}

// udpxProxy passes UDPX connections to UDPX backends.
type udpxProxy struct {
	// Parent
	UDPXDealet_
	// Assocs
	stage   *Stage // current stage
	router  *UDPXRouter
	backend *UDPXBackend // the backend to pass to
	// States
}

func (d *udpxProxy) onCreate(name string, stage *Stage, router *UDPXRouter) {
	d.MakeComp(name)
	d.stage = stage
	d.router = router
}
func (d *udpxProxy) OnShutdown() {
	d.router.DecSub() // dealet
}

func (d *udpxProxy) OnConfigure() {
	// toBackend
	if v, ok := d.Find("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := d.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else if udpxBackend, ok := backend.(*UDPXBackend); ok {
				d.backend = udpxBackend
			} else {
				UseExitf("incorrect backend '%s' for udpxProxy\n", name)
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else {
		UseExitln("toBackend is required for udpxProxy")
	}
}
func (d *udpxProxy) OnPrepare() {
}

func (d *udpxProxy) Deal(conn *UDPXConn) (dealt bool) {
	// TODO
	return true
}
