// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HTTP/3 proxy handlet and WebSocket/3 proxy socklet implementation.

package internal

import (
	"fmt"
)

func init() {
	RegisterHandlet("http3Proxy", func(name string, stage *Stage, app *App) Handlet {
		h := new(http3Proxy)
		h.onCreate(name, stage, app)
		return h
	})
	RegisterSocklet("sock3Proxy", func(name string, stage *Stage, app *App) Socklet {
		s := new(sock3Proxy)
		s.onCreate(name, stage, app)
		return s
	})
}

// http3Proxy handlet passes requests to backend HTTP/3 servers and cache responses.
type http3Proxy struct {
	// Mixins
	httpProxy_
	// States
}

func (h *http3Proxy) onCreate(name string, stage *Stage, app *App) {
	h.httpProxy_.onCreate(name, stage, app)
}
func (h *http3Proxy) OnShutdown() {
	h.app.SubDone()
}

func (h *http3Proxy) OnConfigure() {
	h.httpProxy_.onConfigure(h)
}
func (h *http3Proxy) OnPrepare() {
	h.httpProxy_.onPrepare()
}

func (h *http3Proxy) Handle(req Request, resp Response) (next bool) { // forward or reverse
	var (
		content  any
		content3 any
		err3     error
		conn3    *H3Conn
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
		outgate3 := h.stage.http3
		conn3, err3 = outgate3.FetchConn(req.Authority(), req.IsHTTPS()) // TODO
		if err3 != nil {
			if Debug(1) {
				fmt.Println(err3.Error())
			}
			resp.SendBadGateway(nil)
			return
		}
		defer conn3.closeConn() // TODO
	} else { // reverse
		backend3 := h.backend.(*HTTP3Backend)
		conn3, err3 = backend3.FetchConn()
		if err3 != nil {
			if Debug(1) {
				fmt.Println(err3.Error())
			}
			resp.SendBadGateway(nil)
			return
		}
		defer backend3.StoreConn(conn3)
	}

	stream3 := conn3.FetchStream()
	stream3.onUse(conn3, nil) // TODO
	defer func() {
		stream3.onEnd()
		conn3.StoreStream(stream3)
	}()

	// TODO: use stream3.forwardProxy() or stream3.reverseProxy()

	req3 := stream3.Request()
	if !req3.copyHead(req) {
		stream3.markBroken()
		resp.SendBadGateway(nil)
		return
	}
	hasTrailers := req.hasTrailers()
	if !hasContent || h.bufferClientContent {
		err3 = req3.post(content, hasTrailers) // nil (no content), []byte, TempFile
		if err3 == nil && hasTrailers {
			if !req.walkTrailers(func(name []byte, value []byte) bool {
				return req3.addTrailer(name, value)
			}, true) { // for proxy
				stream3.markBroken()
				err3 = httpAddTrailerFailed
			} else if err3 = req3.finishChunked(); err3 != nil {
				stream3.markBroken()
			}
		} else if hasTrailers {
			stream3.markBroken()
		}
	} else if err3 = req3.pass(req); err3 != nil {
		stream3.markBroken()
	} else if req3.contentSize == -2 { // write last chunk and trailers (if exist)
		if err3 = req3.finishChunked(); err3 != nil {
			stream3.markBroken()
		}
	}
	if err3 != nil {
		resp.SendBadGateway(nil)
		return
	}

	resp3 := stream3.Response()
	for { // until we found a non-1xx status (>= 200)
		//resp3.recvHead()
		if resp3.headResult != StatusOK || resp3.Status() == StatusSwitchingProtocols { // websocket is not served in handlets.
			stream3.markBroken()
			resp.SendBadGateway(nil)
			return
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
			return
		}
		resp3.onEnd()
		resp3.onUse()
	}

	hasContent3 := false
	if req.MethodCode() != MethodHEAD {
		hasContent3 = resp3.HasContent()
	}
	if hasContent3 && h.bufferServerContent { // including size 0
		content3 = resp3.holdContent()
		if content3 == nil {
			// stream3 is marked as broken
			resp.SendBadGateway(nil)
			return
		}
	}

	if !resp.copyHead(resp3) {
		stream3.markBroken()
		return
	}
	hasTrailers3 := resp3.hasTrailers()
	if !hasContent3 || h.bufferServerContent {
		if resp.post(content3, hasTrailers3) != nil { // nil (no content), []byte, TempFile
			if hasTrailers3 {
				stream3.markBroken()
			}
			return
		} else if hasTrailers3 {
			if !resp3.walkTrailers(func(name []byte, value []byte) bool {
				return resp.addTrailer(name, value)
			}, true) { // for proxy
				return
			}
		}
	} else if err := resp.pass(resp3); err != nil {
		stream3.markBroken()
		return
	}
	return
}

// sock3Proxy socklet relays websockets to backend WebSocket/3 servers.
type sock3Proxy struct {
	// Mixins
	sockProxy_
	// States
}

func (s *sock3Proxy) onCreate(name string, stage *Stage, app *App) {
	s.sockProxy_.onCreate(name, stage, app)
}
func (s *sock3Proxy) OnShutdown() {
	s.app.SubDone()
}

func (s *sock3Proxy) OnConfigure() {
	s.sockProxy_.onConfigure(s)
}
func (s *sock3Proxy) OnPrepare() {
	s.sockProxy_.onPrepare()
}

func (s *sock3Proxy) Serve(req Request, sock Socket) { // currently reverse only
	// TODO(diogin): Implementation
	sock.Close()
}
