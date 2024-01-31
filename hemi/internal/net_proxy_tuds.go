// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// TUDS (TCP over Unix Domain Socket) proxy implementation.

package internal

func init() {
	RegisterTCPSDealet("tudsProxy", func(name string, stage *Stage, mesher *TCPSMesher) TCPSDealet {
		d := new(tudsProxy)
		d.onCreate(name, stage, mesher)
		return d
	})
}

// tudsProxy passes TCP/TLS connections to backend TUDS server.
type tudsProxy struct {
	// Mixins
	TCPSDealet_
	// Assocs
	stage   *Stage       // current stage
	mesher  *TCPSMesher  // the mesher to which the dealet belongs
	backend *TUDSBackend // the tuds backend
	// States
}

func (d *tudsProxy) onCreate(name string, stage *Stage, mesher *TCPSMesher) {
	d.MakeComp(name)
	d.stage = stage
	d.mesher = mesher
}
func (d *tudsProxy) OnShutdown() {
	d.mesher.SubDone()
}

func (d *tudsProxy) OnConfigure() {
	// toBackend
	if v, ok := d.Find("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := d.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else if tudsBackend, ok := backend.(*TUDSBackend); ok {
				d.backend = tudsBackend
			} else {
				UseExitf("incorrect backend '%s' for tudsProxy\n", name)
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else {
		UseExitln("toBackend is required for reverse proxy")
	}
}
func (d *tudsProxy) OnPrepare() {
	// Currently nothing.
}

func (d *tudsProxy) Deal(conn *TCPSConn) (dealt bool) {
	// TODO
	return true
}
