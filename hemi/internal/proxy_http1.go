// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HTTP/1 proxy handlet and WebSocket/1 proxy socklet implementation.

package internal

func init() {
	RegisterHandlet("http1Proxy", func(name string, stage *Stage, app *App) Handlet {
		h := new(http1Proxy)
		h.onCreate(name, stage, app)
		return h
	})
	RegisterSocklet("sock1Proxy", func(name string, stage *Stage, app *App) Socklet {
		s := new(sock1Proxy)
		s.onCreate(name, stage, app)
		return s
	})
}

// http1Proxy handlet passes requests to backend HTTP/1 servers and cache responses.
type http1Proxy struct {
	// Mixins
	httpProxy_
	// States
}

func (h *http1Proxy) onCreate(name string, stage *Stage, app *App) {
	h.httpProxy_.onCreate(name, stage, app)
}
func (h *http1Proxy) OnShutdown() {
	h.app.SubDone()
}

func (h *http1Proxy) OnConfigure() {
	h.httpProxy_.onConfigure(h)
}
func (h *http1Proxy) OnPrepare() {
	h.httpProxy_.onPrepare(h)
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
		content = req.HoldContent()
		if content == nil {
			resp.SetStatus(StatusBadRequest)
			resp.SendBytes(nil)
			return
		}
	}

	if h.proxyMode == "forward" {
		outgate1 := h.stage.http1
		conn1, err1 = outgate1.Dial(req.Authority(), req.IsHTTPS()) // TODO: use hostname + colonPort
		if err1 != nil {
			if IsDebug(1) {
				Debugln(err1.Error())
			}
			resp.SendBadGateway(nil)
			return
		}
		defer conn1.closeConn()
	} else { // reverse
		backend1 := h.backend.(*HTTP1Backend)
		conn1, err1 = backend1.FetchConn()
		if err1 != nil {
			if IsDebug(1) {
				Debugln(err1.Error())
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
	if !req1.copyHead(req, h.hostname, h.colonPort) {
		stream1.markBroken()
		resp.SendBadGateway(nil)
		return
	}
	hasTrailers := req.HasTrailers()
	if !hasContent || h.bufferClientContent {
		err1 = req1.post(content, hasTrailers) // nil (no content), []byte, TempFile
		if err1 == nil && hasTrailers {
			if !req.walkTrailers(func(hash uint16, name []byte, value []byte) bool {
				return req1.addTrailer(name, value)
			}) {
				stream1.markBroken()
				err1 = httpAddTrailerFailed
			} else if err1 = req1.finishChunked(); err1 != nil {
				stream1.markBroken()
			}
		} else if hasTrailers {
			stream1.markBroken()
		}
	} else if err1 = req1.pass(req); err1 != nil {
		stream1.markBroken()
	} else if req1.contentSize == -2 { // write last chunk and trailers (if exist)
		if err1 = req1.finishChunked(); err1 != nil {
			stream1.markBroken()
		}
	}
	if err1 != nil {
		resp.SendBadGateway(nil)
		return
	}

	resp1 := stream1.Response()
	for { // until we found a non-1xx status (>= 200)
		resp1.recvHead()
		if resp1.headResult != StatusOK || resp1.Status() == StatusSwitchingProtocols { // websocket is not served in handlets.
			stream1.markBroken()
			resp.SendBadGateway(nil)
			return
		}
		if resp1.Status() >= StatusOK {
			if resp1.keepAlive == 0 {
				conn1.keepConn = false
			}
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
		content1 = resp1.HoldContent()
		if content1 == nil {
			// stream1 is marked as broken
			resp.SendBadGateway(nil)
			return
		}
	}

	if !resp.copyHead(resp1) {
		stream1.markBroken()
		return
	}
	hasTrailers1 := resp1.HasTrailers()
	if !hasContent1 || h.bufferServerContent {
		if resp.post(content1, hasTrailers1) != nil { // nil (no content), []byte, TempFile
			if hasTrailers1 {
				stream1.markBroken()
			}
			return
		} else if hasTrailers1 {
			if !resp1.walkTrailers(func(hash uint16, name []byte, value []byte) bool {
				return resp.addTrailer(name, value)
			}) {
				return
			}
		}
	} else if err := resp.pass(resp1); err != nil {
		stream1.markBroken()
		return
	}
	return
}

// sock1Proxy socklet relays websockets to backend WebSocket/1 servers.
type sock1Proxy struct {
	// Mixins
	sockProxy_
	// States
}

func (s *sock1Proxy) onCreate(name string, stage *Stage, app *App) {
	s.sockProxy_.onCreate(name, stage, app)
}
func (s *sock1Proxy) OnShutdown() {
	s.app.SubDone()
}

func (s *sock1Proxy) OnConfigure() {
	s.sockProxy_.onConfigure(s)
}
func (s *sock1Proxy) OnPrepare() {
	s.sockProxy_.onPrepare(s)
}

func (s *sock1Proxy) Serve(req Request, sock Socket) { // forward or reverse
	// TODO(diogin): Implementation
	sock.Close()
}
