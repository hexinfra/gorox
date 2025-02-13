// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// WebSocket reverse proxy (a.k.a. gateway) implementation.

package hemi

func init() {
	RegisterSocklet("sockProxy", func(compName string, stage *Stage, webapp *Webapp) Socklet {
		s := new(sockProxy)
		s.onCreate(compName, stage, webapp)
		return s
	})
}

// sockProxy socklet passes webSockets to http backends.
type sockProxy struct {
	// Parent
	Socklet_
	// Assocs
	backend HTTPBackend // the *HTTP[1-3]Backend to pass to
	// States
	SOCKProxyConfig // embeded
}

func (s *sockProxy) onCreate(compName string, stage *Stage, webapp *Webapp) {
	s.Socklet_.OnCreate(compName, stage, webapp)
}
func (s *sockProxy) OnShutdown() {
	s.webapp.DecSocklet()
}

func (s *sockProxy) OnConfigure() {
	// .toBackend
	if v, ok := s.Find("toBackend"); ok {
		if compName, ok := v.String(); ok && compName != "" {
			if backend := s.stage.Backend(compName); backend == nil {
				UseExitf("unknown backend: '%s'\n", compName)
			} else {
				s.backend = backend.(HTTPBackend)
			}
		} else {
			UseExitln("invalid toBackend")
		}
	} else {
		UseExitln("toBackend is required for webSocket proxy")
	}
}
func (s *sockProxy) OnPrepare() {
	// Currently nothing.
}

func (s *sockProxy) IsProxy() bool { return true } // works as a reverse proxy

func (s *sockProxy) Serve(req ServerRequest, sock ServerSocket) {
	SOCKReverseProxy(req, sock, s.backend, &s.SOCKProxyConfig)
}

// SOCKProxyConfig
type SOCKProxyConfig struct {
	// TODO
}

// SOCKReverseProxy
func SOCKReverseProxy(servReq ServerRequest, servSock ServerSocket, backend HTTPBackend, proxyConfig *SOCKProxyConfig) {
	// TODO
	servSock.Close()
}
