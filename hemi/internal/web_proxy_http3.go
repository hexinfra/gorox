// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HTTP/3 proxy implementation. See RFC 9114 and 9204.

package internal

func init() {
	RegisterHandlet("http3Proxy", func(name string, stage *Stage, webapp *Webapp) Handlet {
		h := new(http3Proxy)
		h.onCreate(name, stage, webapp)
		return h
	})
	RegisterSocklet("sock3Proxy", func(name string, stage *Stage, webapp *Webapp) Socklet {
		s := new(sock3Proxy)
		s.onCreate(name, stage, webapp)
		return s
	})
}

// http3Proxy handlet passes web requests to another/backend HTTP/3 servers and cache responses.
type http3Proxy struct {
	// Mixins
	exchanProxy_
	// States
}

func (h *http3Proxy) onCreate(name string, stage *Stage, webapp *Webapp) {
	h.exchanProxy_.onCreate(name, stage, webapp)
}
func (h *http3Proxy) OnShutdown() {
	h.webapp.SubDone()
}

func (h *http3Proxy) OnConfigure() {
	h.exchanProxy_.onConfigure()
}
func (h *http3Proxy) OnPrepare() {
	h.exchanProxy_.onPrepare()
}

func (h *http3Proxy) Handle(req Request, resp Response) (handled bool) { // forward or reverse
	var (
		content  any
		conn3    *H3Conn
		err3     error
		content3 any
	)

	hasContent := req.HasContent()
	if hasContent && h.bufferClientContent { // including size 0
		content = req.takeContent()
		if content == nil { // take failed
			// stream is marked as broken
			resp.SetStatus(StatusBadRequest)
			resp.SendBytes(nil)
			return true
		}
	}

	if h.isForward {
		/*
		outgate3 := h.stage.http3Outgate
		conn3, err3 = outgate3.FetchConn(req.Authority(), req.IsHTTPS()) // TODO
		if err3 != nil {
			if Debug() >= 1 {
				Println(err3.Error())
			}
			resp.SendBadGateway(nil)
			return true
		}
		defer conn3.closeConn() // TODO
		*/
	} else { // reverse
		backend3 := h.backend.(*HTTP3Backend)
		conn3, err3 = backend3.FetchConn()
		if err3 != nil {
			if Debug() >= 1 {
				Println(err3.Error())
			}
			resp.SendBadGateway(nil)
			return true
		}
		defer backend3.StoreConn(conn3)
	}

	stream3 := conn3.FetchStream()
	defer conn3.StoreStream(stream3)

	// TODO: use stream3.ForwardProxy() or stream3.ReverseProxy()

	req3 := stream3.Request()
	if !req3.copyHeadFrom(req, h.hostname, h.colonPort, h.viaName, h.addRequestHeaders, h.delRequestHeaders) {
		stream3.markBroken()
		resp.SendBadGateway(nil)
		return true
	}
	if !hasContent || h.bufferClientContent {
		hasTrailers := req.HasTrailers()
		err3 = req3.post(content, hasTrailers) // nil (no content), []byte, tempFile
		if err3 == nil && hasTrailers {
			if !req3.copyTailFrom(req) {
				stream3.markBroken()
				err3 = webOutTrailerFailed
			} else if err3 = req3.endVague(); err3 != nil {
				stream3.markBroken()
			}
		} else if hasTrailers {
			stream3.markBroken()
		}
	} else if err3 = req3.pass(req); err3 != nil {
		stream3.markBroken()
	} else if req3.isVague() { // write last chunk and trailers (if exist)
		if err3 = req3.endVague(); err3 != nil {
			stream3.markBroken()
		}
	}
	if err3 != nil {
		resp.SendBadGateway(nil)
		return true
	}

	resp3 := stream3.Response()
	for { // until we found a non-1xx status (>= 200)
		//resp3.recvHead()
		if resp3.headResult != StatusOK || resp3.Status() == StatusSwitchingProtocols { // websocket is not served in handlets.
			stream3.markBroken()
			if resp3.headResult == StatusRequestTimeout {
				resp.SendGatewayTimeout(nil)
			} else {
				resp.SendBadGateway(nil)
			}
			return true
		}
		if resp3.Status() >= StatusOK {
			break
		}
		// We got 1xx
		// A proxy MUST forward 1xx responses unless the proxy itself requested the generation of the 1xx response.
		// For example, if a proxy adds an "Expect: 100-continue" header field when it forwards a request, then it
		// need not forward the corresponding 100 (Continue) response(s).
		if !resp.pass1xx(resp3) {
			stream3.markBroken()
			return true
		}
		resp3.onEnd()
		resp3.onUse(Version3)
	}

	hasContent3 := false
	if req.MethodCode() != MethodHEAD {
		hasContent3 = resp3.HasContent()
	}
	if hasContent3 && h.bufferServerContent { // including size 0
		content3 = resp3.takeContent()
		if content3 == nil { // take failed
			// stream3 is marked as broken
			resp.SendBadGateway(nil)
			return true
		}
	}

	if !resp.copyHeadFrom(resp3, nil) { // viaName = nil
		stream3.markBroken()
		return true
	}
	if !hasContent3 || h.bufferServerContent {
		hasTrailers3 := resp3.HasTrailers()
		if resp.post(content3, hasTrailers3) != nil { // nil (no content), []byte, tempFile
			if hasTrailers3 {
				stream3.markBroken()
			}
			return true
		} else if hasTrailers3 && !resp.copyTailFrom(resp3) {
			return true
		}
	} else if err := resp.pass(resp3); err != nil {
		stream3.markBroken()
		return true
	}
	return true
}

// sock3Proxy socklet passes websockets to another/backend WebSocket/3 servers.
type sock3Proxy struct {
	// Mixins
	socketProxy_
	// States
}

func (s *sock3Proxy) onCreate(name string, stage *Stage, webapp *Webapp) {
	s.socketProxy_.onCreate(name, stage, webapp)
}
func (s *sock3Proxy) OnShutdown() {
	s.webapp.SubDone()
}

func (s *sock3Proxy) OnConfigure() {
	s.socketProxy_.onConfigure()
}
func (s *sock3Proxy) OnPrepare() {
	s.socketProxy_.onPrepare()
}

func (s *sock3Proxy) Serve(req Request, sock Socket) { // forward or reverse
	// TODO(diogin): Implementation
	if s.isForward {
	} else {
	}
	sock.Close()
}
