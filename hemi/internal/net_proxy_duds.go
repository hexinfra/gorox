// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// DUDS (datagram unix domain socket) proxy implementation.

package internal

func init() {
	RegisterUDPSDealer("dudsProxy", func(name string, stage *Stage, router *UDPSRouter) UDPSDealer {
		d := new(dudsProxy)
		d.onCreate(name, stage, router)
		return d
	})
}

// dudsProxy passes UDP/DTLS links to backend DUDS server.
type dudsProxy struct {
	// Mixins
	UDPSDealer_
	// Assocs
	stage   *Stage
	router  *UDPSRouter
	backend *DUDSBackend
	// States
}

func (d *dudsProxy) onCreate(name string, stage *Stage, router *UDPSRouter) {
	d.MakeComp(name)
	d.stage = stage
	d.router = router
}
func (d *dudsProxy) OnShutdown() {
	d.router.SubDone()
}

func (d *dudsProxy) OnConfigure() {
	// toBackend
	if v, ok := d.Find("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := d.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else if dudsBackend, ok := backend.(*DUDSBackend); ok {
				d.backend = dudsBackend
			} else {
				UseExitf("incorrect backend '%s' for dudsProxy\n", name)
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else {
		UseExitln("toBackend is required for dudsProxy")
	}
}
func (d *dudsProxy) OnPrepare() {
}

func (d *dudsProxy) Deal(link *UDPSLink) (next bool) { // reverse only
	// TODO
	return
}
