// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HTTP/1 proxy implementation.

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

// http1Proxy handlet passes web requests to another/backend HTTP/1 servers and cache responses.
type http1Proxy struct {
	// Mixins
	exchanProxy_
	// States
}

func (h *http1Proxy) onCreate(name string, stage *Stage, app *App) {
	h.exchanProxy_.onCreate(name, stage, app)
}
func (h *http1Proxy) OnShutdown() {
	h.app.SubDone()
}

func (h *http1Proxy) OnConfigure() {
	h.exchanProxy_.onConfigure()
}
func (h *http1Proxy) OnPrepare() {
	h.exchanProxy_.onPrepare()
}

func (h *http1Proxy) Handle(req Request, resp Response) (next bool) { // forward or reverse
	var (
		content  any
		conn1    *H1Conn
		err1     error
		content1 any
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
		outgate1 := h.stage.http1Outgate
		conn1, err1 = outgate1.Dial(req.Hostname()+req.ColonPort(), req.IsHTTPS())
		if err1 != nil {
			if IsDebug(1) {
				Println(err1.Error())
			}
			resp.SendBadGateway(nil)
			return
		}
		defer conn1.Close()
	} else { // reverse
		backend1 := h.backend.(*HTTP1Backend)
		conn1, err1 = backend1.FetchConn()
		if err1 != nil {
			if IsDebug(1) {
				Println(err1.Error())
			}
			resp.SendBadGateway(nil)
			return
		}
		defer backend1.StoreConn(conn1)
	}

	stream1 := conn1.UseStream()
	defer conn1.EndStream(stream1)

	// TODO: use stream1.ForwardProxy() or stream1.ReverseProxy()

	req1 := stream1.Request()
	if !req1.copyHeadFrom(req, h.hostname, h.colonPort, h.viaName) {
		stream1.markBroken()
		resp.SendBadGateway(nil)
		return
	}
	if !hasContent || h.bufferClientContent {
		hasTrailers := req.HasTrailers()
		err1 = req1.post(content, hasTrailers) // nil (no content), []byte, tempFile
		if err1 == nil && hasTrailers {
			if !req1.copyTailFrom(req) {
				stream1.markBroken()
				err1 = webOutTrailerFailed
			} else if err1 = req1.endUnsized(); err1 != nil {
				stream1.markBroken()
			}
		} else if hasTrailers {
			stream1.markBroken()
		}
	} else if err1 = req1.pass(req); err1 != nil {
		stream1.markBroken()
	} else if req1.isUnsized() { // write last chunk and trailers (if exist)
		if err1 = req1.endUnsized(); err1 != nil {
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
			if resp1.headResult == StatusRequestTimeout {
				resp.SendGatewayTimeout(nil)
			} else {
				resp.SendBadGateway(nil)
			}
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
		resp1.onUse(Version1_1)
	}

	hasContent1 := false
	if req.MethodCode() != MethodHEAD {
		hasContent1 = resp1.HasContent()
	}
	if hasContent1 && h.bufferServerContent { // including size 0
		content1 = resp1.takeContent()
		if content1 == nil { // take failed
			// stream1 is marked as broken
			resp.SendBadGateway(nil)
			return
		}
	}

	if !resp.copyHeadFrom(resp1, nil) { // viaName = nil
		stream1.markBroken()
		return
	}
	if !hasContent1 || h.bufferServerContent {
		hasTrailers1 := resp1.HasTrailers()
		if resp.post(content1, hasTrailers1) != nil { // nil (no content), []byte, tempFile
			if hasTrailers1 {
				stream1.markBroken()
			}
			return
		} else if hasTrailers1 && !resp.copyTailFrom(resp1) {
			return
		}
	} else if err := resp.pass(resp1); err != nil {
		stream1.markBroken()
		return
	}
	return
}

// sock1Proxy socklet passes websockets to another/backend WebSocket/1 servers.
type sock1Proxy struct {
	// Mixins
	socketProxy_
	// States
}

func (s *sock1Proxy) onCreate(name string, stage *Stage, app *App) {
	s.socketProxy_.onCreate(name, stage, app)
}
func (s *sock1Proxy) OnShutdown() {
	s.app.SubDone()
}

func (s *sock1Proxy) OnConfigure() {
	s.socketProxy_.onConfigure()
}
func (s *sock1Proxy) OnPrepare() {
	s.socketProxy_.onPrepare()
}

func (s *sock1Proxy) Serve(req Request, sock Socket) { // forward or reverse
	// TODO(diogin): Implementation
	if s.isForward {
	} else {
	}
	sock.Close()
}