// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HTTP/1 proxy handler and WebSocket/1 proxy socklet implementation.

package internal

import (
	"fmt"
)

func init() {
	RegisterHandler("http1Proxy", func(name string, stage *Stage, app *App) Handler {
		h := new(http1Proxy)
		h.init(name, stage, app)
		return h
	})
	RegisterSocklet("sock1Proxy", func(name string, stage *Stage, app *App) Socklet {
		s := new(sock1Proxy)
		s.init(name, stage, app)
		return s
	})
}

// http1Proxy handler adapts and passes requests to backend HTTP/1 servers and cache responses.
type http1Proxy struct {
	// Mixins
	httpProxy_
	// States
}

func (h *http1Proxy) init(name string, stage *Stage, app *App) {
	h.httpProxy_.init(name, stage, app)
}

func (h *http1Proxy) OnConfigure() {
	h.httpProxy_.onConfigure(h)
}
func (h *http1Proxy) OnPrepare() {
	h.httpProxy_.onPrepare(h)
}
func (h *http1Proxy) OnShutdown() {
	h.httpProxy_.onShutdown(h)
}

func (h *http1Proxy) Handle(req Request, resp Response) (next bool) { // forward or reverse
	var (
		content  any
		content1 any
		err1     error
		conn1    *H1Conn
	)

	hasContent := req.HasContent()
	if hasContent && h.bufferClientContent { // including size 0
		content = req.holdContent()
		if content == nil {
			resp.SetStatus(StatusBadRequest)
			resp.SendBytes(nil)
			return
		}
	}

	if h.proxyMode == "forward" {
		outgate1 := h.stage.http1
		conn1, err1 = outgate1.FetchConn(req.Authority(), req.IsHTTPS()) // TODO: use hostname + colonPort
		if err1 != nil {
			if Debug() >= 1 {
				fmt.Println(err1.Error())
			}
			resp.SendBadGateway(nil)
			return
		}
		defer outgate1.StoreConn(conn1)
	} else { // reverse
		backend1 := h.backend.(*HTTP1Backend)
		conn1, err1 = backend1.FetchConn()
		if err1 != nil {
			if Debug() >= 1 {
				fmt.Println(err1.Error())
			}
			resp.SendBadGateway(nil)
			return
		}
		defer backend1.StoreConn(conn1)
	}

	stream1 := conn1.Stream()
	stream1.onUse(conn1)
	defer stream1.onEnd()

	// TODO: use stream1.forwardProxy() or stream1.reverseProxy()

	req1 := stream1.Request()
	if !req1.copyHead(req) {
		stream1.markBroken()
		resp.SendBadGateway(nil)
		return
	}
	if !hasContent || h.bufferClientContent {
		err1 = req1.post(content) // nil (no content), []byte, TempFile
	} else if err1 = req1.pass(req); err1 != nil {
		stream1.markBroken()
	}
	if err1 != nil {
		resp.SendBadGateway(nil)
		return
	}

	resp1 := stream1.Response()
	for { // until we found a non-1xx status (>= 200)
		resp1.recvHead()
		if resp1.headResult != StatusOK || resp1.Status() == StatusSwitchingProtocols { // websocket is not served in handlers.
			stream1.markBroken()
			resp.SendBadGateway(nil)
			return
		}
		if resp1.Status() >= StatusOK {
			break
		}
		// We got 1xx
		if req.VersionCode() == Version1_0 {
			stream1.markBroken()
			resp.SendBadGateway(nil)
			return
		}
		// A proxy MUST forward 1xx responses unless the proxy itself requested the generation of the 1xx response.
		// For example, if a proxy adds an "Expect: 100-continue" header field when it forwards a request, then it
		// need not forward the corresponding 100 (Continue) response(s).
		if !resp.pass1xx(resp1) {
			stream1.markBroken()
			return
		}
		resp1.onEnd()
		resp1.onUse()
	}

	hasContent1 := false
	if req.MethodCode() != MethodHEAD {
		hasContent1 = resp1.HasContent()
	}
	if hasContent1 && h.bufferServerContent { // including size 0
		content1 = resp1.holdContent()
		if content1 == nil {
			// stream1 is marked as broken
			resp.SendBadGateway(nil)
			return
		}
	}

	if !resp.copyHead(resp1) {
		stream1.markBroken()
	} else if !hasContent1 || h.bufferServerContent {
		resp.post(content1) // nil (no content), []byte, TempFile
	} else if err := resp.pass(resp1); err != nil {
		stream1.markBroken()
	}
	return
}

// sock1Proxy socklet passes websockets to backend WebSocket/1 servers.
type sock1Proxy struct {
	// Mixins
	sockProxy_
	// States
}

func (s *sock1Proxy) init(name string, stage *Stage, app *App) {
	s.sockProxy_.init(name, stage, app)
}

func (s *sock1Proxy) OnConfigure() {
	s.sockProxy_.onConfigure(s)
}
func (s *sock1Proxy) OnPrepare() {
	s.sockProxy_.onPrepare(s)
}
func (s *sock1Proxy) OnShutdown() {
	s.sockProxy_.onShutdown(s)
}

func (s *sock1Proxy) Serve(req Request, sock Socket) { // currently reverse only
	// TODO(diogin): Implementation
	sock.Close()
}
