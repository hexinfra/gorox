// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// SUDS (stream unix domain socket) proxy implementation.

package internal

func init() {
	RegisterTCPSDealer("sudsProxy", func(name string, stage *Stage, router *TCPSRouter) TCPSDealer {
		d := new(sudsProxy)
		d.onCreate(name, stage, router)
		return d
	})
}

// sudsProxy passes TCP/TLS connections to backend SUDS server.
type sudsProxy struct {
	// Mixins
	TCPSDealer_
	// Assocs
	stage   *Stage       // current stage
	router  *TCPSRouter  // the router to which the dealer belongs
	backend *SUDSBackend // the suds backend
	// States
}

func (d *sudsProxy) onCreate(name string, stage *Stage, router *TCPSRouter) {
	d.MakeComp(name)
	d.stage = stage
	d.router = router
}
func (d *sudsProxy) OnShutdown() {
	d.router.SubDone()
}

func (d *sudsProxy) OnConfigure() {
	// toBackend
	if v, ok := d.Find("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := d.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else if sudsBackend, ok := backend.(*SUDSBackend); ok {
				d.backend = sudsBackend
			} else {
				UseExitf("incorrect backend '%s' for sudsProxy\n", name)
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else {
		UseExitln("toBackend is required for reverse proxy")
	}
}
func (d *sudsProxy) OnPrepare() {
	// Currently nothing.
}

func (d *sudsProxy) Deal(conn *TCPSConn) (next bool) { // reverse only
	// TODO
	xConn, err := d.backend.Dial()
	if err != nil {
		return
	}
	defer xConn.Close()
	return false
}
