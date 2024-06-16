// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// TCPS (TCP/TLS/UDS) reverse proxy.

package hemi

func init() {
	RegisterTCPSDealet("tcpsProxy", func(name string, stage *Stage, router *TCPSRouter) TCPSDealet {
		d := new(tcpsProxy)
		d.onCreate(name, stage, router)
		return d
	})
}

// tcpsProxy passes TCPS connections to TCPS backends.
type tcpsProxy struct {
	// Parent
	TCPSDealet_
	// Assocs
	stage   *Stage       // current stage
	router  *TCPSRouter  // the router to which the dealet belongs
	backend *TCPSBackend // the backend to pass to
	// States
}

func (d *tcpsProxy) onCreate(name string, stage *Stage, router *TCPSRouter) {
	d.MakeComp(name)
	d.stage = stage
	d.router = router
}
func (d *tcpsProxy) OnShutdown() {
	d.router.DecSub()
}

func (d *tcpsProxy) OnConfigure() {
	// toBackend
	if v, ok := d.Find("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := d.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else if tcpsBackend, ok := backend.(*TCPSBackend); ok {
				d.backend = tcpsBackend
			} else {
				UseExitf("incorrect backend '%s' for tcpsProxy\n", name)
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else {
		UseExitln("toBackend is required for tcpsProxy proxy")
	}
}
func (d *tcpsProxy) OnPrepare() {
	// Currently nothing.
}

func (d *tcpsProxy) Deal(conn *TCPSConn) (dealt bool) {
	// TODO
	return true
}
