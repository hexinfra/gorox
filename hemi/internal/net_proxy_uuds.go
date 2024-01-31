// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// UUDS (UDP over Unix Domain Socket) proxy implementation.

package internal

func init() {
	RegisterUDPSDealet("uudsProxy", func(name string, stage *Stage, mesher *UDPSMesher) UDPSDealet {
		d := new(uudsProxy)
		d.onCreate(name, stage, mesher)
		return d
	})
}

// uudsProxy passes UDP/DTLS links to backend UUDS server.
type uudsProxy struct {
	// Mixins
	UDPSDealet_
	// Assocs
	stage   *Stage
	mesher  *UDPSMesher
	backend *UUDSBackend
	// States
}

func (d *uudsProxy) onCreate(name string, stage *Stage, mesher *UDPSMesher) {
	d.MakeComp(name)
	d.stage = stage
	d.mesher = mesher
}
func (d *uudsProxy) OnShutdown() {
	d.mesher.SubDone()
}

func (d *uudsProxy) OnConfigure() {
	// toBackend
	if v, ok := d.Find("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := d.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else if uudsBackend, ok := backend.(*UUDSBackend); ok {
				d.backend = uudsBackend
			} else {
				UseExitf("incorrect backend '%s' for uudsProxy\n", name)
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else {
		UseExitln("toBackend is required for uudsProxy")
	}
}
func (d *uudsProxy) OnPrepare() {
}

func (d *uudsProxy) Deal(link *UDPSLink) (next bool) { // reverse only
	// TODO
	return
}
