// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// UDPS (UDP/TLS/UDS) reverse proxy.

package hemi

func init() {
	RegisterUDPSDealet("udpsProxy", func(name string, stage *Stage, router *UDPSRouter) UDPSDealet {
		d := new(udpsProxy)
		d.onCreate(name, stage, router)
		return d
	})
}

// udpsProxy passes UDPS connections to UDPS backends.
type udpsProxy struct {
	// Parent
	UDPSDealet_
	// Assocs
	stage   *Stage // current stage
	router  *UDPSRouter
	backend *UDPSBackend // the backend to pass to
	// States
}

func (d *udpsProxy) onCreate(name string, stage *Stage, router *UDPSRouter) {
	d.MakeComp(name)
	d.stage = stage
	d.router = router
}
func (d *udpsProxy) OnShutdown() {
	d.router.DecSub()
}

func (d *udpsProxy) OnConfigure() {
	// toBackend
	if v, ok := d.Find("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := d.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else if udpsBackend, ok := backend.(*UDPSBackend); ok {
				d.backend = udpsBackend
			} else {
				UseExitf("incorrect backend '%s' for udpsProxy\n", name)
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else {
		UseExitln("toBackend is required for udpsProxy")
	}
}
func (d *udpsProxy) OnPrepare() {
}

func (d *udpsProxy) Deal(conn *UDPSConn) (dealt bool) {
	// TODO
	return true
}
