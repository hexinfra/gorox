// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// TCP/TLS relay filter implementation.

package internal

func init() {
	RegisterTCPSFilter("tcpsRelay", func(name string, stage *Stage, mesher *TCPSMesher) TCPSFilter {
		f := new(tcpsRelay)
		f.onCreate(name, stage, mesher)
		return f
	})
}

// tcpsRelay passes TCP/TLS connections to another TCP/TLS server.
type tcpsRelay struct {
	// Mixins
	TCPSFilter_
	// Assocs
	stage   *Stage
	mesher  *TCPSMesher
	backend *TCPSBackend
	// States
	proxyMode string
}

func (f *tcpsRelay) onCreate(name string, stage *Stage, mesher *TCPSMesher) {
	f.MakeComp(name)
	f.stage = stage
	f.mesher = mesher
}
func (f *tcpsRelay) OnShutdown() {
	f.mesher.SubDone()
}

func (f *tcpsRelay) OnConfigure() {
	// proxyMode
	if v, ok := f.Find("proxyMode"); ok {
		if mode, ok := v.String(); ok && (mode == "socks" || mode == "https" || mode == "reverse") {
			f.proxyMode = mode
		} else {
			UseExitln("invalid proxyMode")
		}
	} else {
		f.proxyMode = "reverse"
	}
	// toBackend
	if v, ok := f.Find("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := f.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else if tcpsBackend, ok := backend.(*TCPSBackend); ok {
				f.backend = tcpsBackend
			} else {
				UseExitf("incorrect backend '%s' for tcpsRelay\n", name)
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else if f.proxyMode == "reverse" {
		UseExitln("toBackend is required for reverse proxy")
	}
}
func (f *tcpsRelay) OnPrepare() {
}

func (f *tcpsRelay) Deal(conn *TCPSConn) (next bool) {
	// TODO
	return
}

func (f *tcpsRelay) dealSOCKS(conn *TCPSConn) {
	// TODO: SOCKS 5
}
func (f *tcpsRelay) dealHTTPS(conn *TCPSConn) {
	// TODO: HTTP CONNECT
}
func (f *tcpsRelay) deal(conn *TCPSConn) {
	// TODO: reverse
}
