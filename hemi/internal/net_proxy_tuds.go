// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// TUDS (TCP over Unix Domain Socket) proxy implementation.

package internal

func init() {
	RegisterTCPSFilter("tudsProxy", func(name string, stage *Stage, mesher *TCPSMesher) TCPSFilter {
		f := new(tudsProxy)
		f.onCreate(name, stage, mesher)
		return f
	})
}

// tudsProxy passes TCP/TLS connections to backend TUDS server.
type tudsProxy struct {
	// Mixins
	TCPSFilter_
	// Assocs
	stage   *Stage       // current stage
	mesher  *TCPSMesher  // the mesher to which the filter belongs
	backend *TUDSBackend // the tuds backend
	// States
}

func (f *tudsProxy) onCreate(name string, stage *Stage, mesher *TCPSMesher) {
	f.MakeComp(name)
	f.stage = stage
	f.mesher = mesher
}
func (f *tudsProxy) OnShutdown() {
	f.mesher.SubDone()
}

func (f *tudsProxy) OnConfigure() {
	// toBackend
	if v, ok := f.Find("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := f.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else if tudsBackend, ok := backend.(*TUDSBackend); ok {
				f.backend = tudsBackend
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
func (f *tudsProxy) OnPrepare() {
	// Currently nothing.
}

func (f *tudsProxy) OnSetup(conn *TCPSConn) (next bool) {
	// TODO
	return
}
func (f *tudsProxy) OnInput(buf *Buffer, end bool) (next bool) {
	// TODO
	return
}
func (f *tudsProxy) OnOutput(buf *Buffer, end bool) (next bool) {
	// TODO
	return
}
