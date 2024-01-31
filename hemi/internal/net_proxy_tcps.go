// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// TCP/TLS proxy implementation.

package internal

func init() {
	RegisterTCPSDealet("tcpsProxy", func(name string, stage *Stage, mesher *TCPSMesher) TCPSDealet {
		d := new(tcpsProxy)
		d.onCreate(name, stage, mesher)
		return d
	})
}

// tcpsProxy passes TCP/TLS connections to another/backend TCP/TLS server.
type tcpsProxy struct {
	// Mixins
	TCPSDealet_
	// Assocs
	stage   *Stage       // current stage
	mesher  *TCPSMesher  // the mesher to which the dealet belongs
	backend *TCPSBackend // if works as forward proxy, this is nil
	// States
}

func (d *tcpsProxy) onCreate(name string, stage *Stage, mesher *TCPSMesher) {
	d.MakeComp(name)
	d.stage = stage
	d.mesher = mesher
}
func (d *tcpsProxy) OnShutdown() {
	d.mesher.SubDone()
}

func (d *tcpsProxy) OnConfigure() {
	isReverse := true
	// proxyMode
	if v, ok := d.Find("proxyMode"); ok {
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
	if v, ok := d.Find("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := d.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else if tcpsBackend, ok := backend.(*TCPSBackend); ok {
				d.backend = tcpsBackend
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
func (d *tcpsProxy) OnPrepare() {
	// Currently nothing.
}

func (d *tcpsProxy) Deal(conn *TCPSConn) (dealt bool) {
	// TODO
	return true
}
