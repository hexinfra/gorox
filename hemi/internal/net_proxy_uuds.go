// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// UUDS (UDP over Unix Domain Socket) proxy implementation.

package internal

func init() {
	RegisterUDPSFilter("uudsProxy", func(name string, stage *Stage, mesher *UDPSMesher) UDPSFilter {
		f := new(uudsProxy)
		f.onCreate(name, stage, mesher)
		return f
	})
}

// uudsProxy passes UDP/DTLS links to backend UUDS server.
type uudsProxy struct {
	// Mixins
	UDPSFilter_
	// Assocs
	stage   *Stage
	mesher  *UDPSMesher
	backend *UUDSBackend
	// States
}

func (f *uudsProxy) onCreate(name string, stage *Stage, mesher *UDPSMesher) {
	f.MakeComp(name)
	f.stage = stage
	f.mesher = mesher
}
func (f *uudsProxy) OnShutdown() {
	f.mesher.SubDone()
}

func (f *uudsProxy) OnConfigure() {
	// toBackend
	if v, ok := f.Find("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := f.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else if uudsBackend, ok := backend.(*UUDSBackend); ok {
				f.backend = uudsBackend
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
func (f *uudsProxy) OnPrepare() {
}

func (f *uudsProxy) Deal(link *UDPSLink) (next bool) { // reverse only
	// TODO
	return
}
