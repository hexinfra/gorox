// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// TCP/TLS relay implementation.

package internal

func init() {
	RegisterTCPSDealer("tcpsRelay", func(name string, stage *Stage, router *TCPSRouter) TCPSDealer {
		d := new(tcpsRelay)
		d.onCreate(name, stage, router)
		return d
	})
}

// tcpsRelay passes TCP/TLS connections to another/backend TCP/TLS server.
type tcpsRelay struct {
	// Mixins
	TCPSDealer_
	// Assocs
	stage   *Stage       // current stage
	router  *TCPSRouter  // the router to which the dealer belongs
	backend *TCPSBackend // if works as forward proxy, this is nil
	// States
	process func(*TCPSConn)
}

func (d *tcpsRelay) onCreate(name string, stage *Stage, router *TCPSRouter) {
	d.MakeComp(name)
	d.stage = stage
	d.router = router
}
func (d *tcpsRelay) OnShutdown() {
	d.router.SubDone()
}

func (d *tcpsRelay) OnConfigure() {
	d.process = d.relay
	isReverse := true
	// relayMode
	if v, ok := d.Find("relayMode"); ok {
		if mode, ok := v.String(); ok {
			switch mode {
			case "socks": // SOCKS
				d.process = d.socks
				isReverse = false
			case "https": // HTTP CONNECT
				d.process = d.https
				isReverse = false
			}
		} else {
			UseExitln("invalid relayMode")
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
				UseExitf("incorrect backend '%s' for tcpsRelay\n", name)
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else if isReverse {
		UseExitln("toBackend is required for reverse proxy")
	}
}
func (d *tcpsRelay) OnPrepare() {
	// Currently nothing.
}

func (d *tcpsRelay) Deal(conn *TCPSConn) (next bool) { // forward or reverse
	d.process(conn)
	return false
}

func (d *tcpsRelay) socks(conn *TCPSConn) { // SOCKS
	// TODO
}
func (d *tcpsRelay) https(conn *TCPSConn) { // HTTP CONNECT
	// TODO
}

func (d *tcpsRelay) relay(conn *TCPSConn) { // reverse
	// TODO
	tConn, err := d.backend.Dial()
	if err != nil {
		return
	}
	defer tConn.Close()
}
