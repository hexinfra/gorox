// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// TCP/TLS proxy implementation.

package internal

func init() {
	RegisterTCPSFilter("tcpsProxy", func(name string, stage *Stage, mesher *TCPSMesher) TCPSFilter {
		f := new(tcpsProxy)
		f.onCreate(name, stage, mesher)
		return f
	})
}

// tcpsProxy passes TCP/TLS connections to another/backend TCP/TLS server.
type tcpsProxy struct {
	// Mixins
	TCPSFilter_
	// Assocs
	stage   *Stage       // current stage
	mesher  *TCPSMesher  // the mesher to which the filter belongs
	backend *TCPSBackend // if works as forward proxy, this is nil
	// States
}

func (f *tcpsProxy) onCreate(name string, stage *Stage, mesher *TCPSMesher) {
	f.MakeComp(name)
	f.stage = stage
	f.mesher = mesher
}
func (f *tcpsProxy) OnShutdown() {
	f.mesher.SubDone()
}

func (f *tcpsProxy) OnConfigure() {
	isReverse := true
	// proxyMode
	if v, ok := f.Find("proxyMode"); ok {
		if mode, ok := v.String(); ok {
			switch mode {
			case "socks": // SOCKS
				isReverse = false
			case "https": // HTTP CONNECT
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
				UseExitf("incorrect backend '%s' for tcpsProxy\n", name)
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else if isReverse {
		UseExitln("toBackend is required for reverse proxy")
	}
}
func (f *tcpsProxy) OnPrepare() {
	// Currently nothing.
}

func (f *tcpsProxy) OnSetup(conn *TCPSConn) (next bool) {
	// TODO
	return
}
func (f *tcpsProxy) OnInput(buf *Buffer, end bool) (next bool) {
	// TODO
	return
}
func (f *tcpsProxy) OnOutput(buf *Buffer, end bool) (next bool) {
	// TODO
	return
}
