// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// UDPX (UDP/UDS) reverse proxy implementation.

package hemi

func init() {
	RegisterUDPXDealet("udpxProxy", func(name string, stage *Stage, router *UDPXRouter) UDPXDealet {
		d := new(udpxProxy)
		d.onCreate(name, stage, router)
		return d
	})
}

// udpxProxy dealet passes UDPX connections to UDPX backends.
type udpxProxy struct {
	// Parent
	UDPXDealet_
	// Assocs
	stage   *Stage       // current stage
	router  *UDPXRouter  // the router to which the dealet belongs
	backend *UDPXBackend // the backend to pass to
	// States
	UDPXProxyConfig // embeded
}

func (d *udpxProxy) onCreate(name string, stage *Stage, router *UDPXRouter) {
	d.MakeComp(name)
	d.stage = stage
	d.router = router
}
func (d *udpxProxy) OnShutdown() {
	d.router.DecSub() // dealet
}

func (d *udpxProxy) OnConfigure() {
	// toBackend
	if v, ok := d.Find("toBackend"); ok {
		if name, ok := v.String(); ok && name != "" {
			if backend := d.stage.Backend(name); backend == nil {
				UseExitf("unknown backend: '%s'\n", name)
			} else if udpxBackend, ok := backend.(*UDPXBackend); ok {
				d.backend = udpxBackend
			} else {
				UseExitf("incorrect backend '%s' for udpxProxy\n", name)
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else {
		UseExitln("toBackend is required for udpxProxy")
	}
}
func (d *udpxProxy) OnPrepare() {
}

func (d *udpxProxy) DealWith(conn *UDPXConn) (dealt bool) {
	UDPXReverseProxy(conn, d.backend, &d.UDPXProxyConfig)
	return true
}

// UDPXProxyConfig
type UDPXProxyConfig struct {
	// TODO
}

// UDPXReverseProxy
func UDPXReverseProxy(foreConn *UDPXConn, backend *UDPXBackend, proxyConfig *UDPXProxyConfig) {
	// TODO
}
