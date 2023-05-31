// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Stream UDS proxy implementation.

package internal

func init() {
	RegisterTCPSDealer("unixProxy", func(name string, stage *Stage, router *TCPSRouter) TCPSDealer {
		d := new(unixProxy)
		d.onCreate(name, stage, router)
		return d
	})
}

// unixProxy passes TCP/TLS connections to backend Stream UDS server.
type unixProxy struct {
	// Mixins
	TCPSDealer_
	// Assocs
	stage   *Stage       // current stage
	router  *TCPSRouter  // the router to which the dealer belongs
	backend *UNIXBackend // the unix backend
	// States
}

func (d *unixProxy) onCreate(name string, stage *Stage, router *TCPSRouter) {
	d.MakeComp(name)
	d.stage = stage
	d.router = router
}
func (d *unixProxy) OnShutdown() {
	d.router.SubDone()
}

func (d *unixProxy) OnConfigure() {
	// toBackend
	if v, ok := d.Find("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := d.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else if unixBackend, ok := backend.(*UNIXBackend); ok {
				d.backend = unixBackend
			} else {
				UseExitf("incorrect backend '%s' for unixProxy\n", name)
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else {
		UseExitln("toBackend is required for reverse proxy")
	}
}
func (d *unixProxy) OnPrepare() {
	// Currently nothing.
}

func (d *unixProxy) Deal(conn *TCPSConn) (next bool) { // forward or reverse
	// TODO
	xConn, err := d.backend.Dial()
	if err != nil {
		return
	}
	defer xConn.Close()
	return false
}
