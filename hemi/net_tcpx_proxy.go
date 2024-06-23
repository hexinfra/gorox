// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// TCPX (TCP/TLS/UDS) reverse proxy.

package hemi

func init() {
	RegisterTCPXDealet("tcpxProxy", func(name string, stage *Stage, router *TCPXRouter) TCPXDealet {
		d := new(tcpxProxy)
		d.onCreate(name, stage, router)
		return d
	})
}

// tcpxProxy passes TCPX connections to TCPX backends.
type tcpxProxy struct {
	// Parent
	TCPXDealet_
	// Assocs
	stage   *Stage       // current stage
	router  *TCPXRouter  // the router to which the dealet belongs
	backend *TCPXBackend // the backend to pass to
	// States
}

func (d *tcpxProxy) onCreate(name string, stage *Stage, router *TCPXRouter) {
	d.MakeComp(name)
	d.stage = stage
	d.router = router
}
func (d *tcpxProxy) OnShutdown() {
	d.router.DecSub() // dealet
}

func (d *tcpxProxy) OnConfigure() {
	// toBackend
	if v, ok := d.Find("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := d.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else if tcpxBackend, ok := backend.(*TCPXBackend); ok {
				d.backend = tcpxBackend
			} else {
				UseExitf("incorrect backend '%s' for tcpxProxy\n", name)
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else {
		UseExitln("toBackend is required for tcpxProxy proxy")
	}
}
func (d *tcpxProxy) OnPrepare() {
	// Currently nothing.
}

func (d *tcpxProxy) Deal(conn *TCPXConn) (dealt bool) {
	dealt = true
	tConn, err := d.backend.Dial()
	if err != nil {
		conn.Close()
		return
	}
	go d.relayInbound(conn, tConn)
	d.relayOutbound(tConn, conn)
	conn.wait()
	return
}
func (d *tcpxProxy) relayInbound(tcpxConn *TCPXConn, backConn *TConn) {
	var (
		payload []byte
		tcpxErr error
		backErr error
	)
	for {
		if tcpxErr = tcpxConn.SetReadDeadline(); tcpxErr == nil {
			if payload, tcpxErr = tcpxConn.Recv(); len(payload) > 0 {
				if backErr = backConn.setWriteDeadline(); backErr == nil {
					backErr = backConn.send(payload)
				}
			}
		}
		if tcpxErr != nil || backErr != nil {
			tcpxConn.CloseRead()
			backConn.closeWrite()
			break
		}
	}
	tcpxConn.done()
}
func (d *tcpxProxy) relayOutbound(backConn *TConn, tcpxConn *TCPXConn) {
	var (
		payload []byte
		backErr error
		tcpxErr error
	)
	for {
		if backErr = backConn.setReadDeadline(); backErr == nil {
			if payload, backErr = backConn.recv(); len(payload) > 0 {
				if tcpxErr = tcpxConn.SetWriteDeadline(); tcpxErr == nil {
					tcpxErr = tcpxConn.Send(payload)
				}
			}
		}
		if backErr != nil || tcpxErr != nil {
			backConn.closeRead()
			tcpxConn.CloseWrite()
			break
		}
	}
}
