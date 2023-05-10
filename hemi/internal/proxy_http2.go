// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HTTP/2 proxy implementation.

package internal

func init() {
	RegisterHandlet("http2Proxy", func(name string, stage *Stage, app *App) Handlet {
		h := new(http2Proxy)
		h.onCreate(name, stage, app)
		return h
	})
	RegisterSocklet("sock2Proxy", func(name string, stage *Stage, app *App) Socklet {
		s := new(sock2Proxy)
		s.onCreate(name, stage, app)
		return s
	})
}

// http2Proxy handlet passes requests to another/backend HTTP/2 servers and cache responses.
type http2Proxy struct {
	// Mixins
	normalProxy_
	// States
}

func (h *http2Proxy) onCreate(name string, stage *Stage, app *App) {
	h.normalProxy_.onCreate(name, stage, app)
}
func (h *http2Proxy) OnShutdown() {
	h.app.SubDone()
}

func (h *http2Proxy) OnConfigure() {
	h.normalProxy_.onConfigure()
}
func (h *http2Proxy) OnPrepare() {
	h.normalProxy_.onPrepare()
}

func (h *http2Proxy) Handle(req Request, resp Response) (next bool) { // forward or reverse
	var (
		content  any
		conn2    *H2Conn
		err2     error
		content2 any
	)

	hasContent := req.HasContent()
	if hasContent && h.bufferClientContent { // including size 0
		content = req.takeContent()
		if content == nil { // take failed
			// stream is marked as broken
			resp.SetStatus(StatusBadRequest)
			resp.SendBytes(nil)
			return
		}
	}

	if h.isForward {
		outgate2 := h.stage.http2Outgate
		conn2, err2 = outgate2.FetchConn(req.Authority(), req.IsHTTPS()) // TODO
		if err2 != nil {
			if IsDebug(1) {
				Debugln(err2.Error())
			}
			resp.SendBadGateway(nil)
			return
		}
		defer conn2.closeConn() // TODO
	} else { // reverse
		backend2 := h.backend.(*HTTP2Backend)
		conn2, err2 = backend2.FetchConn()
		if err2 != nil {
			if IsDebug(1) {
				Debugln(err2.Error())
			}
			resp.SendBadGateway(nil)
			return
		}
		defer backend2.StoreConn(conn2)
	}

	stream2 := conn2.FetchStream()
	defer conn2.StoreStream(stream2)

	// TODO: use stream2.ForwardProxy() or stream2.ReverseProxy()

	req2 := stream2.Request()
	if !req2.copyHeadFrom(req, h.hostname, h.colonPort, h.viaName) {
		stream2.markBroken()
		resp.SendBadGateway(nil)
		return
	}
	if !hasContent || h.bufferClientContent {
		hasTrailers := req.HasTrailers()
		err2 = req2.post(content, hasTrailers) // nil (no content), []byte, tempFile
		if err2 == nil && hasTrailers {
			if !req2.copyTailFrom(req) {
				stream2.markBroken()
				err2 = webOutTrailerFailed
			} else if err2 = req2.endUnsized(); err2 != nil {
				stream2.markBroken()
			}
		} else if hasTrailers {
			stream2.markBroken()
		}
	} else if err2 = req2.pass(req); err2 != nil {
		stream2.markBroken()
	} else if req2.isUnsized() { // write last chunk and trailers (if exist)
		if err2 = req2.endUnsized(); err2 != nil {
			stream2.markBroken()
		}
	}
	if err2 != nil {
		resp.SendBadGateway(nil)
		return
	}

	resp2 := stream2.Response()
	for { // until we found a non-1xx status (>= 200)
		//resp2.recvHead()
		if resp2.headResult != StatusOK || resp2.Status() == StatusSwitchingProtocols { // websocket is not served in handlets.
			stream2.markBroken()
			if resp2.headResult == StatusRequestTimeout {
				resp.SendGatewayTimeout(nil)
			} else {
				resp.SendBadGateway(nil)
			}
			return
		}
		if resp2.Status() >= StatusOK {
			break
		}
		// We got 1xx
		// A proxy MUST forward 1xx responses unless the proxy itself requested the generation of the 1xx response.
		// For example, if a proxy adds an "Expect: 100-continue" header field when it forwards a request, then it
		// need not forward the corresponding 100 (Continue) response(s).
		if !resp.pass1xx(resp2) {
			stream2.markBroken()
			return
		}
		resp2.onEnd()
		resp2.onUse(Version2)
	}

	hasContent2 := false
	if req.MethodCode() != MethodHEAD {
		hasContent2 = resp2.HasContent()
	}
	if hasContent2 && h.bufferServerContent { // including size 0
		content2 = resp2.takeContent()
		if content2 == nil { // take failed
			// stream2 is marked as broken
			resp.SendBadGateway(nil)
			return
		}
	}

	if !resp.copyHeadFrom(resp2, nil) { // viaName = nil
		stream2.markBroken()
		return
	}
	if !hasContent2 || h.bufferServerContent {
		hasTrailers2 := resp2.HasTrailers()
		if resp.post(content2, hasTrailers2) != nil { // nil (no content), []byte, tempFile
			if hasTrailers2 {
				stream2.markBroken()
			}
			return
		} else if hasTrailers2 && !resp.copyTailFrom(resp2) {
			return
		}
	} else if err := resp.pass(resp2); err != nil {
		stream2.markBroken()
		return
	}
	return
}

// sock2Proxy socklet passes websockets to another/backend WebSocket/2 servers.
type sock2Proxy struct {
	// Mixins
	socketProxy_
	// States
}

func (s *sock2Proxy) onCreate(name string, stage *Stage, app *App) {
	s.socketProxy_.onCreate(name, stage, app)
}
func (s *sock2Proxy) OnShutdown() {
	s.app.SubDone()
}

func (s *sock2Proxy) OnConfigure() {
	s.socketProxy_.onConfigure()
}
func (s *sock2Proxy) OnPrepare() {
	s.socketProxy_.onPrepare()
}

func (s *sock2Proxy) Serve(req Request, sock Socket) { // forward or reverse
	// TODO(diogin): Implementation
	if s.isForward {
	} else {
	}
	sock.Close()
}
