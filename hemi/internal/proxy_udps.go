// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// UDP/DTLS proxy implementation.

package internal

func init() {
	RegisterUDPSDealer("udpsProxy", func(name string, stage *Stage, mesher *UDPSMesher) UDPSDealer {
		d := new(udpsProxy)
		d.onCreate(name, stage, mesher)
		return d
	})
}

// udpsProxy passes UDP/DTLS connections to backend UDP/DTLS server.
type udpsProxy struct {
	// Mixins
	UDPSDealer_
	// Assocs
	stage   *Stage
	mesher  *UDPSMesher
	backend *UDPSBackend
	// States
}

func (d *udpsProxy) onCreate(name string, stage *Stage, mesher *UDPSMesher) {
	d.MakeComp(name)
	d.stage = stage
	d.mesher = mesher
}
func (d *udpsProxy) OnShutdown() {
	d.mesher.SubDone()
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

func (d *udpsProxy) Deal(conn *UDPSConn) (next bool) { // reverse only
	// TODO
	return
}
