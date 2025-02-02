// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// TCPX (TCP/TLS/UDS) reverse proxy implementation.

package hemi

// TCPXProxyConfig
type TCPXProxyConfig struct {
	// Inbound
	// Outbound
}

// TCPXReverseProxy
func TCPXReverseProxy(servConn *TCPXConn, backend *TCPXBackend, proxyConfig *TCPXProxyConfig) {
	backConn, err := backend.Dial()
	if err != nil {
		servConn.Close()
		return
	}
	inboundOver := make(chan struct{}, 1)
	// Pass inbound data
	go func() {
		var (
			servErr error
			backErr error
			inData  []byte
		)
		for {
			if servErr = servConn.SetReadDeadline(); servErr == nil {
				if inData, servErr = servConn.Recv(); len(inData) > 0 {
					if backErr = backConn.SetWriteDeadline(); backErr == nil {
						backErr = backConn.Send(inData)
					}
				}
			}
			if servErr != nil || backErr != nil {
				servConn.CloseRead()
				backConn.CloseWrite()
				break
			}
		}
		inboundOver <- struct{}{}
	}()
	// Pass outbound data
	var (
		backErr error
		servErr error
		outData []byte
	)
	for {
		if backErr = backConn.SetReadDeadline(); backErr == nil {
			if outData, backErr = backConn.Recv(); len(outData) > 0 {
				if servErr = servConn.SetWriteDeadline(); servErr == nil {
					servErr = servConn.Send(outData)
				}
			}
		}
		if backErr != nil || servErr != nil {
			backConn.CloseRead()
			servConn.CloseWrite()
			break
		}
	}
	<-inboundOver
}

func init() {
	RegisterTCPXDealet("tcpxProxy", func(compName string, stage *Stage, router *TCPXRouter) TCPXDealet {
		d := new(tcpxProxy)
		d.onCreate(compName, stage, router)
		return d
	})
}

// tcpxProxy dealet passes TCPX connections to TCPX backends.
type tcpxProxy struct {
	// Parent
	TCPXDealet_
	// Assocs
	router  *TCPXRouter  // the router to which the dealet belongs
	backend *TCPXBackend // the backend to pass to
	// States
	TCPXProxyConfig // embeded
}

func (d *tcpxProxy) onCreate(compName string, stage *Stage, router *TCPXRouter) {
	d.TCPXDealet_.OnCreate(compName, stage)
	d.router = router
}
func (d *tcpxProxy) OnShutdown() {
	d.router.DecSub() // dealet
}

func (d *tcpxProxy) OnConfigure() {
	// .toBackend
	if v, ok := d.Find("toBackend"); ok {
		if compName, ok := v.String(); ok && compName != "" {
			if backend := d.stage.Backend(compName); backend == nil {
				UseExitf("unknown backend: '%s'\n", compName)
			} else if tcpxBackend, ok := backend.(*TCPXBackend); ok {
				d.backend = tcpxBackend
			} else {
				UseExitf("incorrect backend '%s' for tcpxProxy\n", compName)
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

func (d *tcpxProxy) DealWith(conn *TCPXConn) (dealt bool) {
	TCPXReverseProxy(conn, d.backend, &d.TCPXProxyConfig)
	return true
}
