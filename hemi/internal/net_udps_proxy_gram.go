// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Datagram UDS proxy implementation.

package internal

func init() {
	RegisterUDPSDealer("gramProxy", func(name string, stage *Stage, router *UDPSRouter) UDPSDealer {
		d := new(gramProxy)
		d.onCreate(name, stage, router)
		return d
	})
}

// gramProxy passes UDP/DTLS links to backend Datagram UDS server.
type gramProxy struct {
	// Mixins
	UDPSDealer_
	// Assocs
	stage   *Stage
	router  *UDPSRouter
	backend *GRAMBackend
	// States
}

func (d *gramProxy) onCreate(name string, stage *Stage, router *UDPSRouter) {
	d.MakeComp(name)
	d.stage = stage
	d.router = router
}
func (d *gramProxy) OnShutdown() {
	d.router.SubDone()
}

func (d *gramProxy) OnConfigure() {
	// toBackend
	if v, ok := d.Find("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := d.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else if gramBackend, ok := backend.(*GRAMBackend); ok {
				d.backend = gramBackend
			} else {
				UseExitf("incorrect backend '%s' for gramProxy\n", name)
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else {
		UseExitln("toBackend is required for gramProxy")
	}
}
func (d *gramProxy) OnPrepare() {
}

func (d *gramProxy) Deal(link *UDPSLink) (next bool) { // reverse only
	// TODO
	return
}
