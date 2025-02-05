// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// HTTP/1.x server implementation. See RFC 9112.

// For HTTP/1.x servers, both HTTP/1.0 and HTTP/1.1 are supported. Pipelining is supported but not optimized because it's rarely used.

package hemi

import (
	"bytes"
	"crypto/tls"
	"net"
	"sync"
	"syscall"
	"time"
)

// server1Conn is the server-side HTTP/1.x connection.
type server1Conn struct {
	// Parent
	http1Conn_
	// Mixins
	_serverConn_[*httpxGate]
	// Assocs
	stream server1Stream // an http/1.x connection has exactly one stream
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	closeSafe bool // if false, send a FIN first to avoid TCP's RST following immediate close(). true by default
	// Conn states (zeros)
}

var poolServer1Conn sync.Pool

func getServer1Conn(id int64, gate *httpxGate, netConn net.Conn, rawConn syscall.RawConn) *server1Conn {
	var servConn *server1Conn
	if x := poolServer1Conn.Get(); x == nil {
		servConn = new(server1Conn)
		servStream := &servConn.stream
		servReq, servResp := &servStream.request, &servStream.response
		servReq.stream = servStream
		servReq.in = servReq
		servResp.stream = servStream
		servResp.out = servResp
		servResp.request = servReq
	} else {
		servConn = x.(*server1Conn)
	}
	servConn.onGet(id, gate, netConn, rawConn)
	return servConn
}
func putServer1Conn(servConn *server1Conn) {
	servConn.onPut()
	poolServer1Conn.Put(servConn)
}

func (c *server1Conn) onGet(id int64, gate *httpxGate, netConn net.Conn, rawConn syscall.RawConn) {
	c.http1Conn_.onGet(id, gate, netConn, rawConn)
	c._serverConn_.onGet(gate)

	c.closeSafe = true

	// Input is conn scoped but put in stream scoped request for convenience
	servReq := &c.stream.request
	servReq.input = servReq.stockInput[:]
}
func (c *server1Conn) onPut() {
	// Input, inputNext, and inputEdge are conn scoped but put in stream scoped request for convenience
	servReq := &c.stream.request
	if cap(servReq.input) != cap(servReq.stockInput) { // fetched from pool
		PutNK(servReq.input)
		servReq.input = nil
	}
	servReq.inputNext, servReq.inputEdge = 0, 0

	c._serverConn_.onPut()
	c.gate = nil // put here due to Go's limitation
	c.http1Conn_.onPut()
}

func (c *server1Conn) manager() { // runner
	stream := &c.stream
	for c.persistent { // each queued stream
		stream.onUse(c.nextStreamID(), c)
		stream.execute()
		stream.onEnd()
	}

	// RFC 9112 (section 9.6):
	// If a server performs an immediate close of a TCP connection, there is
	// a significant risk that the client will not be able to read the last
	// HTTP response. If the server receives additional data from the
	// client on a fully closed connection, such as another request sent by
	// the client before receiving the server's response, the server's TCP
	// stack will send a reset packet to the client; unfortunately, the
	// reset packet might erase the client's unacknowledged input buffers
	// before they can be read and interpreted by the client's HTTP parser.

	// To avoid the TCP reset problem, servers typically close a connection
	// in stages. First, the server performs a half-close by closing only
	// the write side of the read/write connection. The server then
	// continues to read from the connection until it receives a
	// corresponding close by the client, or until the server is reasonably
	// certain that its own TCP stack has received the client's
	// acknowledgement of the packet(s) containing the server's last
	// response. Finally, the server fully closes the connection.
	netConn := c.netConn
	if !c.closeSafe {
		// TODO: use "CloseWriter" interface?
		if c.UDSMode() {
			netConn.(*net.UnixConn).CloseWrite()
		} else if c.TLSMode() {
			netConn.(*tls.Conn).CloseWrite()
		} else {
			netConn.(*net.TCPConn).CloseWrite()
		}
		time.Sleep(time.Second)
	}
	netConn.Close()

	c.gate.DecConcurrentConns()
	c.gate.DecSubConn()
	putServer1Conn(c)
}

// server1Stream is the server-side HTTP/1.x stream.
type server1Stream struct {
	// Parent
	http1Stream_[*server1Conn]
	// Mixins
	_serverStream_
	// Assocs
	request  server1Request  // the server-side http/1.x request
	response server1Response // the server-side http/1.x response
	socket   *server1Socket  // the server-side http/1.x webSocket
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (s *server1Stream) onUse(id int64, conn *server1Conn) { // for non-zeros
	s.http1Stream_.onUse(id, conn)
	s._serverStream_.onUse()

	s.request.onUse()
	s.response.onUse()
}
func (s *server1Stream) onEnd() { // for zeros
	s.response.onEnd()
	s.request.onEnd()
	if s.socket != nil {
		s.socket.onEnd()
		s.socket = nil
	}

	s._serverStream_.onEnd()
	s.http1Stream_.onEnd()
	s.conn = nil // we can't do this in http1Stream_.onEnd() due to Go's limit, so put here
}

func (s *server1Stream) execute() {
	req, resp := &s.request, &s.response

	req.recvHead()

	if req.HeadResult() != StatusOK { // receiving request error
		s._serveAbnormal(req, resp)
		return
	}

	if req.IsCONNECT() {
		req.headResult, req.failReason = StatusNotImplemented, "http tunnel proxy is not implemented here"
		s._serveAbnormal(req, resp)
		return
	}

	conn := s.conn
	server := conn.gate.server

	// RFC 9112 (section 3.3):
	// If the server's configuration provides for a fixed URI scheme, or a
	// scheme is provided by a trusted outbound gateway, that scheme is
	// used for the target URI. This is common in large-scale deployments
	// because a gateway server will receive the client's connection context
	// and replace that with their own connection to the inbound server.
	// Otherwise, if the request is received over a secured connection, the
	// target URI's scheme is "https"; if not, the scheme is "http".
	if server.forceScheme != -1 { // forceScheme is set explicitly
		req.schemeCode = uint8(server.forceScheme)
	} else if server.alignScheme { // scheme is not forced. should it be aligned?
		if conn.TLSMode() { // secured
			if req.schemeCode == SchemeHTTP {
				req.schemeCode = SchemeHTTPS
			}
		} else { // not secured
			if req.schemeCode == SchemeHTTPS {
				req.schemeCode = SchemeHTTP
			}
		}
	}

	webapp := server.findWebapp(req.UnsafeHostname())

	if webapp == nil {
		req.headResult, req.failReason = StatusNotFound, "target webapp is not found in this server"
		s._serveAbnormal(req, resp)
		return
	}
	if !webapp.isDefault && !bytes.Equal(req.UnsafeColonport(), server.ColonportBytes()) {
		req.headResult, req.failReason = StatusNotFound, "authoritative webapp is not found in this server"
		s._serveAbnormal(req, resp)
		return
	}

	req.webapp = webapp
	resp.webapp = webapp

	if !req.upgradeSocket { // exchan mode
		if req.contentIsForm() {
			if req.formKind == httpFormMultipart { // we allow a larger content size for uploading through multipart/form-data (because large files are written to disk).
				req.maxContentSize = webapp.maxMultiformSize
			} else { // application/x-www-form-urlencoded is limited in a smaller size because it will be loaded into memory
				req.maxContentSize = int64(req.maxMemoryContentSize)
			}
		}
		if req.contentSize > req.maxContentSize {
			if req.expectContinue {
				req.headResult = StatusExpectationFailed
			} else {
				req.headResult, req.failReason = StatusContentTooLarge, "content size exceeds webapp's limit"
			}
			s._serveAbnormal(req, resp)
			return
		}

		// Prepare the response according to the request
		if req.IsHEAD() {
			resp.forbidContent = true
		}

		if req.expectContinue && !s._writeContinue() {
			return
		}
		conn.cumulativeStreams.Add(1)
		if maxCumulativeStreams := server.maxCumulativeStreamsPerConn; (maxCumulativeStreams > 0 && conn.cumulativeStreams.Load() == maxCumulativeStreams) || !req.KeepAlive() || conn.gate.IsShut() {
			conn.persistent = false // reaches limit, or client told us to close, or gate was shut
		}

		s.executeExchan(webapp, req, resp)

		if s.isBroken() {
			conn.persistent = false // i/o error, close anyway
		}
	} else { // socket mode.
		if req.expectContinue && !s._writeContinue() {
			return
		}

		s.executeSocket()

		conn.persistent = false // explicitly close for webSocket
	}
}
func (s *server1Stream) _serveAbnormal(req *server1Request, resp *server1Response) { // 4xx & 5xx
	if DebugLevel() >= 2 {
		Printf("server=%s gate=%d conn=%d headResult=%d\n", s.conn.gate.server.CompName(), s.conn.gate.ID(), s.conn.id, s.request.headResult)
	}
	s.conn.persistent = false // we are in abnormal state, so close anyway

	status := req.headResult
	if status == -1 || (status == StatusRequestTimeout && !req.gotSomeInput) {
		return // send nothing.
	}
	// So we need to send something...
	if status == StatusContentTooLarge || status == StatusURITooLong || status == StatusRequestHeaderFieldsTooLarge {
		// The receiving side may has data when we close the connection
		s.conn.closeSafe = false
	}
	var content []byte
	if errorPage, ok := serverErrorPages[status]; !ok {
		content = http1Controls[status]
	} else if req.failReason == "" {
		content = errorPage
	} else {
		content = ConstBytes(req.failReason)
	}
	// Use response as a dumb struct here, don't use its methods (like Send) to send anything as we are in abnormal state!
	resp.status = status
	resp.AddHeaderBytes(bytesContentType, bytesTypeHTML)
	resp.contentSize = int64(len(content))
	if status == StatusMethodNotAllowed {
		// Currently only WebSocket use this status in abnormal state, so GET is hard coded.
		resp.AddHeaderBytes(bytesAllow, bytesGET)
	}
	resp.finalizeHeaders()
	if req.IsHEAD() || resp.forbidContent { // we follow the method semantic even we are in abnormal
		resp.vector = resp.fixedVector[0:3]
	} else {
		resp.vector = resp.fixedVector[0:4]
		resp.vector[3] = content
	}
	resp.vector[0] = resp.controlData()
	resp.vector[1] = resp.addedHeaders()
	resp.vector[2] = resp.fixedHeaders()
	if s.setWriteDeadline() == nil { // ignore any error, as the connection will be closed anyway.
		s.writev(&resp.vector)
	}
}
func (s *server1Stream) _writeContinue() bool { // 100 continue
	// This is an interim response, write directly.
	if s.setWriteDeadline() == nil {
		if _, err := s.write(http1BytesContinue); err == nil {
			return true
		}
	}
	s.conn.persistent = false // i/o error, close anyway
	return false
}

func (s *server1Stream) executeExchan(webapp *Webapp, req *server1Request, resp *server1Response) { // request & response
	webapp.dispatchExchan(req, resp)

	if !resp.isSent { // only happens for sized contents because for vague contents the response must be sent on echo()
		resp.sendChain()
	} else if resp.isVague() { // for vague contents, we end vague content and write trailer fields (if exist) here
		resp.endVague()
	}

	if !req.contentReceived { // request content exists but was not used, we receive and drop it here
		req._dropContent()
	}
}
func (s *server1Stream) executeSocket() { // upgrade: websocket. See RFC 6455
	// TODO(diogin): implementation.
	// NOTICE: use idle timeout or clear read timeout otherwise?
	s.write([]byte("HTTP/1.1 501 Not Implemented\r\nConnection: close\r\n\r\n"))
}

// server1Request is the server-side HTTP/1.x request.
type server1Request struct { // incoming. needs parsing
	// Parent
	serverRequest_
	// Assocs
	in1 _http1In_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *server1Request) onUse() {
	r.serverRequest_.onUse(Version1_1)
	r.in1.onUse(&r._httpIn_)
}
func (r *server1Request) onEnd() {
	r.serverRequest_.onEnd()
	r.in1.onEnd()
}

func (r *server1Request) recvHead() { // control data + header section
	// The entire request head must be received in one read timeout
	if err := r.stream.setReadDeadline(); err != nil {
		r.headResult = -1
		return
	}
	if r.inputEdge == 0 && !r.in1.growHead() { // r.inputEdge == 0 means r.input is empty, so we must fill it
		// r.headResult is set.
		return
	}
	if !r._recvControlData() || !r.in1.recvHeaderLines() || !r.examineHead() {
		// r.headResult is set.
		return
	}
	r.tidyInput()
	if DebugLevel() >= 2 {
		Printf("[server1Stream=%d]<-------[%s]\n", r.stream.ID(), r.input[r.head.from:r.head.edge])
	}
}
func (r *server1Request) _recvControlData() bool { // request-line = method SP request-target SP HTTP-version CRLF
	r.elemBack, r.elemFore = 0, 0

	// method = token
	// token = 1*tchar
	methodHash := uint16(0)
	for {
		if b := r.input[r.elemFore]; httpTchar[b] != 0 {
			methodHash += uint16(b)
			if r.elemFore++; r.elemFore == r.inputEdge && !r.in1.growHead() {
				return false
			}
		} else if b == ' ' {
			break
		} else {
			r.headResult, r.failReason = StatusBadRequest, "invalid character in method"
			return false
		}
	}
	if r.elemBack == r.elemFore {
		r.headResult, r.failReason = StatusBadRequest, "empty method"
		return false
	}
	r.gotSomeInput = true
	r.method.set(r.elemBack, r.elemFore)
	r.recognizeMethod(r.input[r.elemBack:r.elemFore], methodHash)
	// Skip SP after method
	if r.elemFore++; r.elemFore == r.inputEdge && !r.in1.growHead() {
		return false
	}

	// Now r.elemFore is at request-target.
	r.elemBack = r.elemFore
	// request-target = absolute-form / origin-form / authority-form / asterisk-form
	if b := r.input[r.elemFore]; b != '*' && !r.IsCONNECT() { // absolute-form / origin-form
		if b != '/' { // absolute-form
			// absolute-form = absolute-URI
			// absolute-URI = scheme ":" hier-part [ "?" query ]
			// scheme = ALPHA *( ALPHA / DIGIT / "+" / "-" / "." )
			// hier-part = "//" authority path-abempty
			// authority = host [ ":" port ]
			// path-abempty = *( "/" segment)

			// Scheme
			for {
				if b := r.input[r.elemFore]; b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '+' || b == '-' || b == '.' {
					// Do nothing
				} else if b >= 'A' && b <= 'Z' {
					// RFC 9110 (section 4.2.3):
					// The scheme and host are case-insensitive and normally provided in lowercase;
					// all other components are compared in a case-sensitive manner.
					r.input[r.elemFore] = b + 0x20 // to lower
				} else if b == ':' {
					break
				} else {
					r.headResult, r.failReason = StatusBadRequest, "bad scheme"
					return false
				}
				if r.elemFore++; r.elemFore == r.inputEdge && !r.in1.growHead() {
					return false
				}
			}
			if scheme := r.input[r.elemBack:r.elemFore]; bytes.Equal(scheme, bytesHTTP) {
				r.schemeCode = SchemeHTTP
			} else if bytes.Equal(scheme, bytesHTTPS) {
				r.schemeCode = SchemeHTTPS
			} else {
				r.headResult, r.failReason = StatusBadRequest, "unknown scheme"
				return false
			}
			// Skip ':'
			if r.elemFore++; r.elemFore == r.inputEdge && !r.in1.growHead() {
				return false
			}
			if r.input[r.elemFore] != '/' {
				r.headResult, r.failReason = StatusBadRequest, "bad first slash"
				return false
			}
			// Skip '/'
			if r.elemFore++; r.elemFore == r.inputEdge && !r.in1.growHead() {
				return false
			}
			if r.input[r.elemFore] != '/' {
				r.headResult, r.failReason = StatusBadRequest, "bad second slash"
				return false
			}
			// Skip '/'
			if r.elemFore++; r.elemFore == r.inputEdge && !r.in1.growHead() {
				return false
			}
			// authority = host [ ":" port ]
			// host = IP-literal / IPv4address / reg-name
			r.elemBack = r.elemFore
			for {
				if b = r.input[r.elemFore]; b >= 'A' && b <= 'Z' {
					r.input[r.elemFore] = b + 0x20 // to lower
				} else if b == '/' || b == ' ' {
					break
				}
				if r.elemFore++; r.elemFore == r.inputEdge && !r.in1.growHead() {
					return false
				}
			}
			if r.elemBack == r.elemFore {
				r.headResult, r.failReason = StatusBadRequest, "empty authority is not allowed"
				return false
			}
			if !r.parseAuthority(r.elemBack, r.elemFore, true) { // save = true
				r.headResult, r.failReason = StatusBadRequest, "bad authority"
				return false
			}
			if b == ' ' { // end of request-target. don't treat this as asterisk-form! r.uri is empty but we fetch it through r.URI() or like which gives '/' if uri is empty
				if r.IsOPTIONS() { // OPTIONS http://www.example.org:8001 HTTP/1.1
					r.asteriskOptions = true
				} else { // GET http://www.example.org HTTP/1.1
					// Do nothing.
				}
				goto beforeVersion // request target is done, since origin-form always starts with '/', while b is ' ' here.
			}
			r.elemBack = r.elemFore // at '/'.
		}
		// RFC 9112 (3.2.1)
		//
		// The most common form of request-target is the origin-form.
		//
		//   origin-form = absolute-path [ "?" query ]
		//       absolute-path = 1*( "/" segment )
		//           segment = *pchar
		//       query = *( pchar / "/" / "?" )
		//
		// When making a request directly to an origin server, other than a
		// CONNECT or server-wide OPTIONS request (as detailed below), a client
		// MUST send only the absolute path and query components of the target
		// URI as the request-target. If the target URI's path component is
		// empty, the client MUST send "/" as the path within the origin-form of
		// request-target. A Host header field is also sent, as defined in
		// Section 7.2 of [HTTP].
		var (
			state = 1   // in path
			octet byte  // byte value of %xx
			qsOff int32 // offset of query string, if exists
		)
		query := &r.mainPair
		query.zero()
		query.kind = pairQuery
		query.place = placeArray // all received queries are placed in r.array because queries are decoded

		// r.elemFore is at '/'.
	uri:
		for { // TODO: use a better algorithm to improve performance, state machine might be slow here.
			b := r.input[r.elemFore]
			switch state {
			case 1: // in path
				if httpPchar[b] == 1 { // excluding '?'
					r.arrayPush(b)
				} else if b == '%' {
					state = 0x1f // '1' means from state 1, 'f' means first HEXDIG
				} else if b == '?' {
					// Path is over, switch to query string parsing
					r.path = r.array[0:r.arrayEdge]
					r.queries.from = uint8(len(r.primes))
					r.queries.edge = r.queries.from
					query.nameFrom = r.arrayEdge
					qsOff = r.elemFore - r.elemBack
					state = 2
				} else if b == ' ' { // end of request-target
					break uri
				} else {
					r.headResult, r.failReason = StatusBadRequest, "invalid path"
					return false
				}
			case 2: // in query string and expecting '=' to get a name
				if b == '=' {
					if nameSize := r.arrayEdge - query.nameFrom; nameSize <= 255 {
						query.nameSize = uint8(nameSize)
						query.value.from = r.arrayEdge
					} else {
						r.headResult, r.failReason = StatusBadRequest, "query name too long"
						return false
					}
					state = 3
				} else if httpPchar[b] > 0 { // including '?'
					if b == '+' {
						b = ' ' // application/x-www-form-urlencoded encodes ' ' as '+'
					}
					query.nameHash += uint16(b)
					r.arrayPush(b)
				} else if b == '%' {
					state = 0x2f // '2' means from state 2, 'f' means first HEXDIG
				} else if b == ' ' { // end of request-target
					break uri
				} else {
					r.headResult, r.failReason = StatusBadRequest, "invalid query name"
					return false
				}
			case 3: // in query string and expecting '&' to get a value
				if b == '&' {
					query.value.edge = r.arrayEdge
					if query.nameSize > 0 && !r.addQuery(query) {
						return false
					}
					query.nameHash = 0 // reset for next query
					query.nameFrom = r.arrayEdge
					state = 2
				} else if httpPchar[b] > 0 { // including '?'
					if b == '+' {
						b = ' ' // application/x-www-form-urlencoded encodes ' ' as '+'
					}
					r.arrayPush(b)
				} else if b == '%' {
					state = 0x3f // '3' means from state 3, 'f' means first HEXDIG
				} else if b == ' ' { // end of request-target
					break uri
				} else {
					r.headResult, r.failReason = StatusBadRequest, "invalid query value"
					return false
				}
			default: // in query string and expecting HEXDIG
				if b == ' ' { // end of request-target
					break uri
				}
				nybble, ok := byteFromHex(b)
				if !ok {
					r.headResult, r.failReason = StatusBadRequest, "invalid pct encoding"
					return false
				}
				if state&0xf == 0xf { // Expecting the first HEXDIG
					octet = nybble << 4
					state &= 0xf0 // this reserves last state and leads to the state of second HEXDIG
				} else { // Expecting the second HEXDIG
					octet |= nybble
					if state == 0x20 { // in name, calculate name hash
						query.nameHash += uint16(octet)
					} else if octet == 0x00 && state == 0x10 { // For security reasons, we reject "\x00" in path.
						r.headResult, r.failReason = StatusBadRequest, "malformed path"
						return false
					}
					r.arrayPush(octet)
					state >>= 4 // restore previous state
				}
			}
			if r.elemFore++; r.elemFore == r.inputEdge && !r.in1.growHead() {
				return false
			}
		}
		if state == 1 { // path ends without a '?'
			r.path = r.array[0:r.arrayEdge]
		} else if state == 2 { // in query string and no '=' found
			r.queryString.set(r.elemBack+qsOff, r.elemFore)
			// Since there is no '=', we ignore this query
		} else if state == 3 { // in query string and no '&' found
			r.queryString.set(r.elemBack+qsOff, r.elemFore)
			query.value.edge = r.arrayEdge
			if query.nameSize > 0 && !r.addQuery(query) {
				return false
			}
		} else { // incomplete pct-encoded
			r.headResult, r.failReason = StatusBadRequest, "incomplete pct-encoded"
			return false
		}

		r.uri.set(r.elemBack, r.elemFore)
		if qsOff == 0 {
			r.encodedPath = r.uri
		} else {
			r.encodedPath.set(r.elemBack, r.elemBack+qsOff)
		}
		r.cleanPath()
	} else if b == '*' { // OPTIONS *, asterisk-form
		// RFC 9112 (section 3.2.4):
		// The "asterisk-form" of request-target is only used for a server-wide OPTIONS request (Section 9.3.7 of [HTTP]).
		if !r.IsOPTIONS() {
			r.headResult, r.failReason = StatusBadRequest, "asterisk-form is only used by OPTIONS method"
			return false
		}
		// Skip '*'. We don't use it as uri! Instead, we use '/'. To test OPTIONS *, test r.asteriskOptions set below.
		if r.elemFore++; r.elemFore == r.inputEdge && !r.in1.growHead() {
			return false
		}
		r.asteriskOptions = true
		// Expect SP
		if r.input[r.elemFore] != ' ' {
			r.headResult, r.failReason = StatusBadRequest, "malformed asterisk-form"
			return false
		}
		// RFC 9112 (section 3.3):
		// If the request-target is in authority-form or asterisk-form, the
		// target URI's combined path and query component is empty. Otherwise,
		// the target URI's combined path and query component is the request-target.
	} else { // CONNECT method, must be authority-form
		// RFC 9112 (section 3.2.3):
		// The "authority-form" of request-target is only used for CONNECT requests (Section 9.3.6 of [HTTP]).
		//
		//   authority-form = uri-host ":" port
		//
		// When making a CONNECT request to establish a tunnel through one or more proxies,
		// a client MUST send only the host and port of the tunnel destination as the request-target.
		for {
			if b := r.input[r.elemFore]; b >= 'A' && b <= 'Z' {
				r.input[r.elemFore] = b + 0x20 // to lower
			} else if b == ' ' {
				break
			}
			if r.elemFore++; r.elemFore == r.inputEdge && !r.in1.growHead() {
				return false
			}
		}
		if r.elemBack == r.elemFore {
			r.headResult, r.failReason = StatusBadRequest, "empty authority is not allowed"
			return false
		}
		if !r.parseAuthority(r.elemBack, r.elemFore, true) { // save = true
			r.headResult, r.failReason = StatusBadRequest, "invalid authority"
			return false
		}
		// RFC 9112 (section 3.3):
		// If the request-target is in authority-form or asterisk-form, the
		// target URI's combined path and query component is empty. Otherwise,
		// the target URI's combined path and query component is the request-target.
	}

beforeVersion: // r.elemFore is at ' '.
	// Skip SP before HTTP-version
	if r.elemFore++; r.elemFore == r.inputEdge && !r.in1.growHead() {
		return false
	}

	// Now r.elemFore is at HTTP-version.
	r.elemBack = r.elemFore
	// HTTP-version = HTTP-name "/" DIGIT "." DIGIT
	// HTTP-name = %x48.54.54.50 ; "HTTP", case-sensitive
	if have := r.inputEdge - r.elemFore; have >= 9 {
		// r.elemFore -> EOL
		// r.inputEdge -> after EOL or more
		r.elemFore += 8
	} else { // have < 9, but len("HTTP/1.X\n") = 9.
		// r.elemFore at 'H' -> EOL
		// r.inputEdge at "TTP/1.X\n" -> after EOL
		r.elemFore = r.inputEdge - 1
		for i, n := int32(0), 9-have; i < n; i++ {
			if r.elemFore++; r.elemFore == r.inputEdge && !r.in1.growHead() {
				return false
			}
		}
	}
	if version := r.input[r.elemBack:r.elemFore]; bytes.Equal(version, bytesHTTP1_1) {
		r.httpVersion = Version1_1
	} else if bytes.Equal(version, bytesHTTP1_0) {
		r.httpVersion = Version1_0
	} else { // i don't believe there will be an HTTP/1.2 in the future.
		r.headResult = StatusHTTPVersionNotSupported
		return false
	}
	if r.input[r.elemFore] == '\r' {
		if r.elemFore++; r.elemFore == r.inputEdge && !r.in1.growHead() {
			return false
		}
	}
	if r.input[r.elemFore] != '\n' {
		r.headResult, r.failReason = StatusBadRequest, "bad eol of start line"
		return false
	}
	r.receiving = httpSectionHeaders
	// Skip '\n'
	if r.elemFore++; r.elemFore == r.inputEdge && !r.in1.growHead() {
		return false
	}

	return true
}
func (r *server1Request) tidyInput() {
	// r.elemFore is at the beginning of content (if exists) or next request (if exists and is pipelined).
	if r.contentSize == -1 { // no content
		r.contentReceived = true      // we treat it as "received"
		r.formReceived = true         // set anyway
		if r.elemFore < r.inputEdge { // still has data, stream is pipelined
			r.inputNext = r.elemFore // mark the beginning of the next request
		} else { // r.elemFore == r.inputEdge, no data anymore
			r.inputNext, r.inputEdge = 0, 0 // reset
		}
		return
	}
	// content exists (sized or vague)
	r.imme.set(r.elemFore, r.inputEdge)
	if r.contentSize >= 0 { // sized mode
		immeSize := int64(r.imme.size())
		if immeSize == 0 || immeSize <= r.contentSize {
			r.inputNext, r.inputEdge = 0, 0 // reset
		}
		if immeSize >= r.contentSize {
			r.contentReceived = true
			edge := r.elemFore + int32(r.contentSize)
			if immeSize > r.contentSize { // still has data, streams are pipelined
				r.imme.set(r.elemFore, edge)
				r.inputNext = edge // mark the beginning of next request
			}
			r.receivedSize = r.contentSize           // content is received entirely.
			r.contentText = r.input[r.elemFore:edge] // exact.
			r.contentTextKind = httpContentTextInput
		}
		if r.contentSize == 0 {
			r.formReceived = true // no content means no form, so mark it as "received"
		}
	} else { // vague mode
		// We don't know the size of vague content. Let chunked receivers to decide & clean r.input.
	}
}

func (r *server1Request) readContent() (data []byte, err error) { return r.in1.readContent() }

// server1Response is the server-side HTTP/1.x response.
type server1Response struct { // outgoing. needs building
	// Parent
	serverResponse_
	// Assocs
	out1 _http1Out_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *server1Response) onUse() {
	r.serverResponse_.onUse(Version1_1)
	r.out1.onUse(&r._httpOut_)
}
func (r *server1Response) onEnd() {
	r.serverResponse_.onEnd()
	r.out1.onEnd()
}

func (r *server1Response) controlData() []byte { // overrides r.serverResponse_.controlData()
	var start []byte
	if r.status < int16(len(http1Controls)) && http1Controls[r.status] != nil {
		start = http1Controls[r.status]
	} else {
		r.start = http1Status
		r.start[9] = byte(r.status/100 + '0')
		r.start[10] = byte(r.status/10%10 + '0')
		r.start[11] = byte(r.status%10 + '0')
		start = r.start[:]
	}
	return start
}

func (r *server1Response) addHeader(name []byte, value []byte) bool {
	return r.out1.addHeader(name, value)
}
func (r *server1Response) header(name []byte) (value []byte, ok bool) { return r.out1.header(name) }
func (r *server1Response) hasHeader(name []byte) bool                 { return r.out1.hasHeader(name) }
func (r *server1Response) delHeader(name []byte) (deleted bool)       { return r.out1.delHeader(name) }
func (r *server1Response) delHeaderAt(i uint8)                        { r.out1.delHeaderAt(i) }

func (r *server1Response) AddHTTPSRedirection(authority string) bool {
	headerSize := len(http1BytesLocationHTTPS)
	if authority == "" {
		headerSize += len(r.request.UnsafeAuthority())
	} else {
		headerSize += len(authority)
	}
	headerSize += len(r.request.UnsafeURI()) + len(bytesCRLF)
	if from, _, ok := r.growHeaders(headerSize); ok {
		from += copy(r.fields[from:], http1BytesLocationHTTPS)
		if authority == "" {
			from += copy(r.fields[from:], r.request.UnsafeAuthority())
		} else {
			from += copy(r.fields[from:], authority)
		}
		from += copy(r.fields[from:], r.request.UnsafeURI())
		r.out1._addCRLFHeader(from)
		return true
	} else {
		return false
	}
}
func (r *server1Response) AddHostnameRedirection(hostname string) bool {
	var prefix []byte
	if r.request.IsHTTPS() {
		prefix = http1BytesLocationHTTPS
	} else {
		prefix = http1BytesLocationHTTP
	}
	headerSize := len(prefix)
	// TODO: remove colonport if colonport is default?
	colonport := r.request.UnsafeColonport()
	headerSize += len(hostname) + len(colonport) + len(r.request.UnsafeURI()) + len(bytesCRLF)
	if from, _, ok := r.growHeaders(headerSize); ok {
		from += copy(r.fields[from:], prefix)
		from += copy(r.fields[from:], hostname) // this is almost always configured, not client provided
		from += copy(r.fields[from:], colonport)
		from += copy(r.fields[from:], r.request.UnsafeURI()) // original uri, won't split the response
		r.out1._addCRLFHeader(from)
		return true
	} else {
		return false
	}
}
func (r *server1Response) AddDirectoryRedirection() bool {
	var prefix []byte
	if r.request.IsHTTPS() {
		prefix = http1BytesLocationHTTPS
	} else {
		prefix = http1BytesLocationHTTP
	}
	req := r.request
	headerSize := len(prefix)
	headerSize += len(req.UnsafeAuthority()) + len(req.UnsafeURI()) + 1 + len(bytesCRLF)
	if from, _, ok := r.growHeaders(headerSize); ok {
		from += copy(r.fields[from:], prefix)
		from += copy(r.fields[from:], req.UnsafeAuthority())
		from += copy(r.fields[from:], req.UnsafeEncodedPath())
		r.fields[from] = '/'
		from++
		if len(req.UnsafeQueryString()) > 0 {
			from += copy(r.fields[from:], req.UnsafeQueryString())
		}
		r.out1._addCRLFHeader(from)
		return true
	} else {
		return false
	}
}

func (r *server1Response) AddCookie(cookie *Cookie) bool {
	if cookie.name == "" || cookie.invalid {
		return false
	}
	headerSize := len(bytesSetCookie) + len(bytesColonSpace) + cookie.size() + len(bytesCRLF) // set-cookie: cookie\r\n
	if from, _, ok := r.growHeaders(headerSize); ok {
		from += copy(r.fields[from:], bytesSetCookie)
		r.fields[from] = ':'
		r.fields[from+1] = ' '
		from += 2
		from += cookie.writeTo(r.fields[from:])
		r.out1._addCRLFHeader(from)
		return true
	} else {
		return false
	}
}

func (r *server1Response) sendChain() error { return r.out1.sendChain() }

func (r *server1Response) echoHeaders() error { return r.out1.writeHeaders() }
func (r *server1Response) echoChain() error   { return r.out1.echoChain(r.request.IsHTTP1_1()) } // chunked only for HTTP/1.1

func (r *server1Response) addTrailer(name []byte, value []byte) bool {
	if r.request.VersionCode() == Version1_1 {
		return r.out1.addTrailer(name, value)
	}
	return true // HTTP/1.0 doesn't support http trailers.
}
func (r *server1Response) trailer(name []byte) (value []byte, ok bool) { return r.out1.trailer(name) }

func (r *server1Response) proxyPass1xx(backResp BackendResponse) bool {
	backResp.proxyDelHopHeaderFields()
	r.status = backResp.Status()
	if !backResp.proxyWalkHeaderLines(r, func(out httpOut, headerLine *pair, headerName []byte, lineValue []byte) bool {
		return out.insertHeader(headerLine.nameHash, headerName, lineValue) // some header fields (e.g. "connection") are restricted
	}) {
		return false
	}
	r.vector = r.fixedVector[0:3]
	r.vector[0] = r.controlData()
	r.vector[1] = r.addedHeaders()
	r.vector[2] = bytesCRLF
	// 1xx response has no content.
	if r.out1.writeVector() != nil {
		return false
	}
	// For next use.
	r.onEnd()
	r.onUse()
	return true
}
func (r *server1Response) proxyPassHeaders() error          { return r.out1.writeHeaders() }
func (r *server1Response) proxyPassBytes(data []byte) error { return r.out1.proxyPassBytes(data) }

func (r *server1Response) finalizeHeaders() { // add at most 256 bytes
	// date: Sun, 06 Nov 1994 08:49:37 GMT\r\n
	if r.iDate == 0 {
		clock := r.stream.(*server1Stream).conn.gate.stage.clock
		r.fieldsEdge += uint16(clock.writeDate1(r.fields[r.fieldsEdge:]))
	}
	// expires: Sun, 06 Nov 1994 08:49:37 GMT\r\n
	if r.unixTimes.expires >= 0 {
		r.fieldsEdge += uint16(clockWriteHTTPDate1(r.fields[r.fieldsEdge:], bytesExpires, r.unixTimes.expires))
	}
	// last-modified: Sun, 06 Nov 1994 08:49:37 GMT\r\n
	if r.unixTimes.lastModified >= 0 {
		r.fieldsEdge += uint16(clockWriteHTTPDate1(r.fields[r.fieldsEdge:], bytesLastModified, r.unixTimes.lastModified))
	}
	conn := r.stream.(*server1Stream).conn
	if r.contentSize != -1 { // with content
		if !r.forbidFraming {
			if !r.isVague() { // content-length: >=0\r\n
				sizeBuffer := r.stream.buffer256() // enough for content-length
				n := i64ToDec(r.contentSize, sizeBuffer)
				r.out1._addFixedHeader(bytesContentLength, sizeBuffer[:n])
			} else if r.request.VersionCode() == Version1_1 { // transfer-encoding: chunked\r\n
				r.fieldsEdge += uint16(copy(r.fields[r.fieldsEdge:], http1BytesTransferChunked))
			} else {
				// RFC 9112 (section 6.1):
				// A server MUST NOT send a response containing Transfer-Encoding unless
				// the corresponding request indicates HTTP/1.1 (or later minor revisions).
				conn.persistent = false // for HTTP/1.0 we have to close the connection anyway since there is no way to delimit the chunks
			}
		}
		// content-type: text/html\r\n
		if r.iContentType == 0 {
			r.fieldsEdge += uint16(copy(r.fields[r.fieldsEdge:], http1BytesContentTypeHTML))
		}
	}
	if conn.persistent { // connection: keep-alive\r\n
		r.fieldsEdge += uint16(copy(r.fields[r.fieldsEdge:], http1BytesConnectionKeepAlive))
	} else { // connection: close\r\n
		r.fieldsEdge += uint16(copy(r.fields[r.fieldsEdge:], http1BytesConnectionClose))
	}
}
func (r *server1Response) finalizeVague() error {
	if r.request.VersionCode() == Version1_1 {
		return r.out1.finalizeVague()
	}
	return nil // HTTP/1.0 does nothing.
}

func (r *server1Response) addedHeaders() []byte { return r.fields[0:r.fieldsEdge] }
func (r *server1Response) fixedHeaders() []byte { return http1BytesFixedResponseHeaders }

// server1Socket is the server-side HTTP/1.x webSocket.
type server1Socket struct { // incoming and outgoing
	// Parent
	serverSocket_
	// Assocs
	so1 _http1Socket_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

var poolServer1Socket sync.Pool

func getServer1Socket(stream *server1Stream) *server1Socket {
	// TODO
	return nil
}
func putServer1Socket(socket *server1Socket) {
	// TODO
}

func (s *server1Socket) onUse() {
	s.serverSocket_.onUse()
	s.so1.onUse(&s._httpSocket_)
}
func (s *server1Socket) onEnd() {
	s.serverSocket_.onEnd()
	s.so1.onEnd()
}

func (s *server1Socket) serverTodo1() {
	s.serverTodo()
	s.so1.todo1()
}
