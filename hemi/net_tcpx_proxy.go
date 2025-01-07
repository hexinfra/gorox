// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// TCPX (TCP/TLS/UDS) reverse proxy implementation.

package hemi

func init() {
	RegisterTCPXDealet("tcpxProxy", func(name string, stage *Stage, router *TCPXRouter) TCPXDealet {
		d := new(tcpxProxy)
		d.onCreate(name, stage, router)
		return d
	})
}

// tcpxProxy dealet passes TCPX connections to TCPX backends.
type tcpxProxy struct {
	// Parent
	TCPXDealet_
	// Assocs
	stage   *Stage       // current stage
	router  *TCPXRouter  // the router to which the dealet belongs
	backend *TCPXBackend // the backend to pass to
	// States
	TCPXProxyConfig // embeded
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
	TCPXReverseProxy(conn, d.backend, &d.TCPXProxyConfig)
	return true
}

// TCPXProxyConfig
type TCPXProxyConfig struct {
	// Inbound
	// Outbound
}

// TCPXReverseProxy
func TCPXReverseProxy(foreConn *TCPXConn, backend *TCPXBackend, proxyConfig *TCPXProxyConfig) {
	backConn, backErr := backend.Dial()
	if backErr != nil {
		foreConn.Close()
		return
	}
	inboundOver := make(chan struct{}, 1)
	// Inbound
	go func() {
		var (
			foreData []byte
			foreErr  error
			backErr  error
		)
		for {
			if foreErr = foreConn.SetReadDeadline(); foreErr == nil {
				if foreData, foreErr = foreConn.Recv(); len(foreData) > 0 {
					if backErr = backConn.SetWriteDeadline(); backErr == nil {
						_, backErr = backConn.Send(foreData)
					}
				}
			}
			if foreErr != nil || backErr != nil {
				foreConn.CloseRead()
				backConn.CloseWrite()
				break
			}
		}
		inboundOver <- struct{}{}
	}()
	// Outbound
	var (
		backData []byte
		foreErr  error
	)
	for {
		if backErr = backConn.SetReadDeadline(); backErr == nil {
			if backData, backErr = backConn.Recv(); len(backData) > 0 {
				if foreErr = foreConn.SetWriteDeadline(); foreErr == nil {
					_, foreErr = foreConn.Send(backData)
				}
			}
		}
		if backErr != nil || foreErr != nil {
			backConn.CloseRead()
			foreConn.CloseWrite()
			break
		}
	}
	<-inboundOver
}
