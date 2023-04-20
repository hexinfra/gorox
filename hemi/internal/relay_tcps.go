// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// TCP/TLS relay dealer implementation.

package internal

func init() {
	RegisterTCPSDealer("tcpsRelay", func(name string, stage *Stage, mesher *TCPSMesher) TCPSDealer {
		f := new(tcpsRelay)
		f.onCreate(name, stage, mesher)
		return f
	})
}

// tcpsRelay passes TCP/TLS connections to another TCP/TLS server.
type tcpsRelay struct {
	// Mixins
	TCPSDealer_
	// Assocs
	stage   *Stage       // current stage
	mesher  *TCPSMesher  // the mesher to which the dealer belongs
	backend *TCPSBackend // if works as forward proxy, this is nil
	// States
	process func(*TCPSConn)
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
	f.process = f.relay
	isReverse := true
	// proxyMode
	if v, ok := f.Find("proxyMode"); ok {
		if mode, ok := v.String(); ok {
			switch mode {
			case "socks": // SOCKS
				f.process = f.socks
				isReverse = false
			case "https": // HTTP CONNECT
				f.process = f.https
				isReverse = false
			}
		} else {
			UseExitln("invalid proxyMode")
		}
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
	} else if isReverse {
		UseExitln("toBackend is required for reverse proxy")
	}
}
func (f *tcpsRelay) OnPrepare() {
	// Currently nothing.
}

func (f *tcpsRelay) Process(conn *TCPSConn) (next bool) {
	f.process(conn)
	return false
}

func (f *tcpsRelay) relay(conn *TCPSConn) { // reverse proxy
	tConn, err := f.backend.Dial()
	if err != nil {
		return
	}
	defer tConn.Close()
}

func (f *tcpsRelay) socks(conn *TCPSConn) { // SOCKS
	// TODO
}
func (f *tcpsRelay) https(conn *TCPSConn) { // HTTP CONNECT
	// TODO
}
