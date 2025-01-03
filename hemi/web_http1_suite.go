// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// HTTP/1.x server and backend implementation. See RFC 9112.
// NOTE: httpxServer and httpxGate are used by both HTTP/1.x and HTTP/2.

// For server, both HTTP/1.0 and HTTP/1.1 are supported. Pipelining is supported but not optimized because it's rarely used.
// For backend, only HTTP/1.1 is used, so HTTP/1.x backends MUST support HTTP/1.1. Pipelining is not used.

package hemi

import (
	"bytes"
	"crypto/tls"
	"io"
	"net"
	"sync"
	"syscall"
	"time"
)

//////////////////////////////////////// HTTP/1.x server implementation ////////////////////////////////////////

// server1Conn is the server-side HTTP/1.x connection.
type server1Conn struct {
	// Parent
	http1Conn_
	// Assocs
	stream server1Stream // an http/1.x connection has exactly one stream
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	gate       *httpxGate // the gate to which the conn belongs
	persistent bool       // keep the connection after current stream? true by default
	closeSafe  bool       // if false, send a FIN first to avoid TCP's RST following immediate close(). true by default
	// Conn states (zeros)
}

var poolServer1Conn sync.Pool

func getServer1Conn(id int64, gate *httpxGate, netConn net.Conn, rawConn syscall.RawConn) *server1Conn {
	var serverConn *server1Conn
	if x := poolServer1Conn.Get(); x == nil {
		serverConn = new(server1Conn)
		stream := &serverConn.stream
		stream.conn = serverConn
		req, resp := &stream.request, &stream.response
		req.stream = stream
		req.inMessage = req
		resp.stream = stream
		resp.outMessage = resp
		resp.request = req
	} else {
		serverConn = x.(*server1Conn)
	}
	serverConn.onGet(id, gate, netConn, rawConn)
	return serverConn
}
func putServer1Conn(serverConn *server1Conn) {
	serverConn.onPut()
	poolServer1Conn.Put(serverConn)
}

func (c *server1Conn) onGet(id int64, gate *httpxGate, netConn net.Conn, rawConn syscall.RawConn) {
	server := gate.server
	c.http1Conn_.onGet(id, server.Stage().ID(), gate.IsUDS(), gate.IsTLS(), netConn, rawConn, server.ReadTimeout(), server.WriteTimeout())

	c.gate = gate
	c.persistent = true
	c.closeSafe = true

	// Input is conn scoped but put in stream scoped request for convenience
	req := &c.stream.request
	req.input = req.stockInput[:]
}
func (c *server1Conn) onPut() {
	// Input, inputNext, and inputEdge are conn scoped but put in stream scoped request for convenience
	req := &c.stream.request
	if cap(req.input) != cap(req.stockInput) { // fetched from pool
		PutNK(req.input)
		req.input = nil
	}
	req.inputNext, req.inputEdge = 0, 0

	c.gate = nil

	c.http1Conn_.onPut()
}

func (c *server1Conn) serve() { // runner
	stream := &c.stream
	for c.persistent { // each queued stream
		stream.onUse()
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
		if c.IsUDS() {
			netConn.(*net.UnixConn).CloseWrite()
		} else if c.IsTLS() {
			netConn.(*tls.Conn).CloseWrite()
		} else {
			netConn.(*net.TCPConn).CloseWrite()
		}
		time.Sleep(time.Second)
	}
	netConn.Close()

	c.gate.DecActives()
	c.gate.DecConn()
	putServer1Conn(c)
}

// server1Stream is the server-side HTTP/1.x stream.
type server1Stream struct {
	// Parent
	http1Stream_[*server1Conn]
	// Assocs
	request  server1Request  // the server-side http/1.x request
	response server1Response // the server-side http/1.x response
	socket   *server1Socket  // the server-side http/1.x webSocket
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (s *server1Stream) onUse() { // for non-zeros
	s.http1Stream_.onUse()

	s.request.onUse(Version1_1)
	s.response.onUse(Version1_1)
}
func (s *server1Stream) onEnd() { // for zeros
	s.response.onEnd()
	s.request.onEnd()
	if s.socket != nil {
		s.socket.onEnd()
		s.socket = nil
	}

	s.http1Stream_.onEnd()
}

func (s *server1Stream) Holder() webHolder { return s.conn.gate.server }

func (s *server1Stream) execute() {
	req, resp := &s.request, &s.response

	req.recvHead()

	if req.HeadResult() != StatusOK { // receiving request error
		s._serveAbnormal(req, resp)
		return
	}

	if req.methodCode == MethodCONNECT {
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
	} else { // scheme is not forced. should it be aligned?
		if conn.IsTLS() { // secured
			if req.schemeCode == SchemeHTTP && server.alignScheme {
				req.schemeCode = SchemeHTTPS
			}
		} else { // not secured
			if req.schemeCode == SchemeHTTPS && server.alignScheme {
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
		if req.formKind != httpFormNotForm { // content is an html form
			if req.formKind == httpFormMultipart { // we allow a larger content size for uploading through multipart/form-data (large files are written to disk).
				req.maxContentSize = webapp.maxMultiformSize
			} else { // application/x-www-form-urlencoded is limited in a smaller size.
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
		if req.methodCode == MethodHEAD {
			resp.forbidContent = true
		}

		if req.expectContinue && !s._writeContinue() {
			return
		}
		conn.usedStreams.Add(1)
		if maxStreams := server.MaxStreamsPerConn(); (maxStreams > 0 && conn.usedStreams.Load() == maxStreams) || req.keepAlive == 0 || conn.gate.IsShut() {
			conn.persistent = false // reaches limit, or client told us to close, or gate was shut
		}

		s.executeExchan(webapp, req, resp)

		if s.isBroken() {
			conn.persistent = false // i/o error
		}
	} else { // socket mode.
		if req.expectContinue && !s._writeContinue() {
			return
		}

		s.executeSocket()

		conn.persistent = false // explicitly
	}
}
func (s *server1Stream) _serveAbnormal(req *server1Request, resp *server1Response) { // 4xx & 5xx
	if DebugLevel() >= 2 {
		Printf("server=%s gate=%d conn=%d headResult=%d\n", s.conn.gate.server.Name(), s.conn.gate.ID(), s.conn.id, s.request.headResult)
	}
	s.conn.persistent = false // close anyway.

	status := req.headResult
	if status == -1 || (status == StatusRequestTimeout && !req.gotInput) {
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
	// Use response as a dumb struct here, don't use its methods (like Send) to send anything!
	resp.status = status
	resp.AddHeaderBytes(bytesContentType, bytesTypeHTMLUTF8)
	resp.contentSize = int64(len(content))
	if status == StatusMethodNotAllowed {
		// Currently only WebSocket use this status in abnormal state, so GET is hard coded.
		resp.AddHeaderBytes(bytesAllow, bytesGET)
	}
	resp.finalizeHeaders()
	if req.methodCode == MethodHEAD || resp.forbidContent { // we follow the method semantic even we are in abnormal
		resp.vector = resp.fixedVector[0:3]
	} else {
		resp.vector = resp.fixedVector[0:4]
		resp.vector[3] = content
	}
	resp.vector[0] = resp.control()
	resp.vector[1] = resp.addedHeaders()
	resp.vector[2] = resp.fixedHeaders()
	// Ignore any error, as the connection will be closed anyway.
	if s.setWriteDeadline() == nil {
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
	// i/o error
	s.conn.persistent = false
	return false
}

func (s *server1Stream) executeExchan(webapp *Webapp, req *server1Request, resp *server1Response) { // request & response
	webapp.dispatchExchan(req, resp)

	if !resp.isSent { // only happens on sized contents because for vague contents the response must be sent on echo()
		resp.sendChain()
	} else if resp.isVague() { // for vague contents, we end vague content and write trailers (if exist) here
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
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *server1Request) recvHead() { // request-line + headers
	// The entire request head must be received in one timeout
	if err := r._beforeRead(&r.recvTime); err != nil {
		r.headResult = -1
		return
	}
	if r.inputEdge == 0 && !r.growHead1() { // r.inputEdge == 0 means r.input is empty, so we must fill it
		// r.headResult is set.
		return
	}
	if !r._recvRequestLine() || !r.recvHeaders1() || !r.examineHead() {
		// r.headResult is set.
		return
	}
	r.cleanInput()
	if DebugLevel() >= 2 {
		Printf("[server1Stream=%d]<------- [%s]\n", r.stream.Conn().ID(), r.input[r.head.from:r.head.edge])
	}
}
func (r *server1Request) _recvRequestLine() bool { // request-line = method SP request-target SP HTTP-version CRLF
	r.elemBack, r.elemFore = 0, 0

	// method = token
	// token = 1*tchar
	methodHash := uint16(0)
	for {
		if b := r.input[r.elemFore]; httpTchar[b] != 0 {
			methodHash += uint16(b)
			if r.elemFore++; r.elemFore == r.inputEdge && !r.growHead1() {
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
	r.gotInput = true
	r.method.set(r.elemBack, r.elemFore)
	r.recognizeMethod(r.input[r.elemBack:r.elemFore], methodHash)
	// Skip SP after method
	if r.elemFore++; r.elemFore == r.inputEdge && !r.growHead1() {
		return false
	}

	// Now r.elemFore is at request-target.
	r.elemBack = r.elemFore
	// request-target = absolute-form / origin-form / authority-form / asterisk-form
	if b := r.input[r.elemFore]; b != '*' && r.methodCode != MethodCONNECT { // absolute-form / origin-form
		if b != '/' { // absolute-form
			r.targetForm = httpTargetAbsolute
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
				if r.elemFore++; r.elemFore == r.inputEdge && !r.growHead1() {
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
			if r.elemFore++; r.elemFore == r.inputEdge && !r.growHead1() {
				return false
			}
			if r.input[r.elemFore] != '/' {
				r.headResult, r.failReason = StatusBadRequest, "bad first slash"
				return false
			}
			// Skip '/'
			if r.elemFore++; r.elemFore == r.inputEdge && !r.growHead1() {
				return false
			}
			if r.input[r.elemFore] != '/' {
				r.headResult, r.failReason = StatusBadRequest, "bad second slash"
				return false
			}
			// Skip '/'
			if r.elemFore++; r.elemFore == r.inputEdge && !r.growHead1() {
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
				if r.elemFore++; r.elemFore == r.inputEdge && !r.growHead1() {
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
			if b == ' ' { // ends of request-target
				// Don't treat this as httpTargetAsterisk! r.uri is empty but we fetch it through r.URI() or like which gives '/' if uri is empty.
				if r.methodCode == MethodOPTIONS {
					// OPTIONS http://www.example.org:8001 HTTP/1.1
					r.asteriskOptions = true
				} else {
					// GET http://www.example.org HTTP/1.1
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
			if r.elemFore++; r.elemFore == r.inputEdge && !r.growHead1() {
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
		r.targetForm = httpTargetAsterisk
		// RFC 9112 (section 3.2.4):
		// The "asterisk-form" of request-target is only used for a server-wide OPTIONS request (Section 9.3.7 of [HTTP]).
		if r.methodCode != MethodOPTIONS {
			r.headResult, r.failReason = StatusBadRequest, "asterisk-form is only used by OPTIONS method"
			return false
		}
		// Skip '*'. We don't use it as uri! Instead, we use '/'. To test OPTIONS *, test r.asteriskOptions set below.
		if r.elemFore++; r.elemFore == r.inputEdge && !r.growHead1() {
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
	} else { // r.methodCode == MethodCONNECT, authority-form
		r.targetForm = httpTargetAuthority
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
			if r.elemFore++; r.elemFore == r.inputEdge && !r.growHead1() {
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
	if r.elemFore++; r.elemFore == r.inputEdge && !r.growHead1() {
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
			if r.elemFore++; r.elemFore == r.inputEdge && !r.growHead1() {
				return false
			}
		}
	}
	if version := r.input[r.elemBack:r.elemFore]; bytes.Equal(version, bytesHTTP1_1) {
		r.httpVersion = Version1_1
	} else if bytes.Equal(version, bytesHTTP1_0) {
		r.httpVersion = Version1_0
	} else { // i don't believe there will be a HTTP/1.2 in the future.
		r.headResult = StatusHTTPVersionNotSupported
		return false
	}
	if r.input[r.elemFore] == '\r' {
		if r.elemFore++; r.elemFore == r.inputEdge && !r.growHead1() {
			return false
		}
	}
	if r.input[r.elemFore] != '\n' {
		r.headResult, r.failReason = StatusBadRequest, "bad eol of start line"
		return false
	}
	r.receiving = httpSectionHeaders
	// Skip '\n'
	if r.elemFore++; r.elemFore == r.inputEdge && !r.growHead1() {
		return false
	}

	return true
}
func (r *server1Request) cleanInput() {
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

func (r *server1Request) readContent() (p []byte, err error) { return r.readContent1() }

// server1Response is the server-side HTTP/1.x response.
type server1Response struct { // outgoing. needs building
	// Parent
	serverResponse_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *server1Response) control() []byte { // HTTP/1.1 NNN X
	var start []byte
	if r.status >= int16(len(http1Controls)) || http1Controls[r.status] == nil {
		r.start = http1Template
		r.start[9] = byte(r.status/100 + '0')
		r.start[10] = byte(r.status/10%10 + '0')
		r.start[11] = byte(r.status%10 + '0')
		start = r.start[:]
	} else {
		start = http1Controls[r.status]
	}
	return start
}

func (r *server1Response) addHeader(name []byte, value []byte) bool   { return r.addHeader1(name, value) }
func (r *server1Response) header(name []byte) (value []byte, ok bool) { return r.header1(name) }
func (r *server1Response) hasHeader(name []byte) bool                 { return r.hasHeader1(name) }
func (r *server1Response) delHeader(name []byte) (deleted bool)       { return r.delHeader1(name) }
func (r *server1Response) delHeaderAt(i uint8)                        { r.delHeaderAt1(i) }

func (r *server1Response) AddHTTPSRedirection(authority string) bool {
	headerSize := len(http1BytesLocationHTTPS)
	if authority == "" {
		headerSize += len(r.request.UnsafeAuthority())
	} else {
		headerSize += len(authority)
	}
	headerSize += len(r.request.UnsafeURI()) + len(bytesCRLF)
	if from, _, ok := r.growHeader(headerSize); ok {
		from += copy(r.fields[from:], http1BytesLocationHTTPS)
		if authority == "" {
			from += copy(r.fields[from:], r.request.UnsafeAuthority())
		} else {
			from += copy(r.fields[from:], authority)
		}
		from += copy(r.fields[from:], r.request.UnsafeURI())
		r._addCRLFHeader1(from)
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
	if from, _, ok := r.growHeader(headerSize); ok {
		from += copy(r.fields[from:], prefix)
		from += copy(r.fields[from:], hostname) // this is almost always configured, not client provided
		from += copy(r.fields[from:], colonport)
		from += copy(r.fields[from:], r.request.UnsafeURI()) // original uri, won't split the response
		r._addCRLFHeader1(from)
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
	if from, _, ok := r.growHeader(headerSize); ok {
		from += copy(r.fields[from:], prefix)
		from += copy(r.fields[from:], req.UnsafeAuthority())
		from += copy(r.fields[from:], req.UnsafeEncodedPath())
		r.fields[from] = '/'
		from++
		if len(req.UnsafeQueryString()) > 0 {
			from += copy(r.fields[from:], req.UnsafeQueryString())
		}
		r._addCRLFHeader1(from)
		return true
	} else {
		return false
	}
}
func (r *server1Response) setConnectionClose() { r.stream.(*server1Stream).conn.persistent = false }

func (r *server1Response) AddCookie(cookie *Cookie) bool {
	if cookie.name == "" || cookie.invalid {
		return false
	}
	headerSize := len(bytesSetCookie) + len(bytesColonSpace) + cookie.size() + len(bytesCRLF) // set-cookie: cookie\r\n
	if from, _, ok := r.growHeader(headerSize); ok {
		from += copy(r.fields[from:], bytesSetCookie)
		r.fields[from] = ':'
		r.fields[from+1] = ' '
		from += 2
		from += cookie.writeTo(r.fields[from:])
		r._addCRLFHeader1(from)
		return true
	} else {
		return false
	}
}

func (r *server1Response) sendChain() error { return r.sendChain1() }

func (r *server1Response) echoHeaders() error { return r.writeHeaders1() }
func (r *server1Response) echoChain() error   { return r.echoChain1(r.request.IsHTTP1_1()) } // chunked only for HTTP/1.1

func (r *server1Response) addTrailer(name []byte, value []byte) bool {
	if r.request.VersionCode() == Version1_1 {
		return r.addTrailer1(name, value)
	}
	return true // HTTP/1.0 doesn't support trailer.
}
func (r *server1Response) trailer(name []byte) (value []byte, ok bool) { return r.trailer1(name) }

func (r *server1Response) proxyPass1xx(backResp response) bool {
	backResp.proxyDelHopHeaders()
	r.status = backResp.Status()
	if !backResp.forHeaders(func(header *pair, name []byte, value []byte) bool {
		return r.insertHeader(header.nameHash, name, value)
	}) {
		return false
	}
	r.vector = r.fixedVector[0:3]
	r.vector[0] = r.control()
	r.vector[1] = r.addedHeaders()
	r.vector[2] = bytesCRLF
	// 1xx response has no content.
	if r.writeVector1() != nil {
		return false
	}
	// For next use.
	r.onEnd()
	r.onUse(Version1_1)
	return true
}
func (r *server1Response) proxyPassHeaders() error       { return r.writeHeaders1() }
func (r *server1Response) proxyPassBytes(p []byte) error { return r.proxyPassBytes1(p) }

func (r *server1Response) finalizeHeaders() { // add at most 256 bytes
	// date: Sun, 06 Nov 1994 08:49:37 GMT\r\n
	if r.iDate == 0 {
		clock := r.stream.(*server1Stream).conn.gate.server.stage.clock
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
				r._addFixedHeader1(bytesContentLength, sizeBuffer[:n])
			} else if r.request.VersionCode() == Version1_1 { // transfer-encoding: chunked\r\n
				r.fieldsEdge += uint16(copy(r.fields[r.fieldsEdge:], http1BytesTransferChunked))
			} else {
				// RFC 9112 (section 6.1):
				// A server MUST NOT send a response containing Transfer-Encoding unless
				// the corresponding request indicates HTTP/1.1 (or later minor revisions).
				conn.persistent = false // close conn anyway for HTTP/1.0
			}
		}
		// content-type: text/html; charset=utf-8\r\n
		if r.iContentType == 0 {
			r.fieldsEdge += uint16(copy(r.fields[r.fieldsEdge:], http1BytesContentTypeHTMLUTF8))
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
		return r.finalizeVague1()
	}
	return nil // HTTP/1.0 does nothing.
}

func (r *server1Response) addedHeaders() []byte { return r.fields[0:r.fieldsEdge] }
func (r *server1Response) fixedHeaders() []byte { return http1BytesFixedResponseHeaders }

// server1Socket is the server-side HTTP/1.x webSocket.
type server1Socket struct { // incoming and outgoing
	// Parent
	serverSocket_
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
}
func (s *server1Socket) onEnd() {
	s.serverSocket_.onEnd()
}

//////////////////////////////////////// HTTP/1.x backend implementation ////////////////////////////////////////

func init() {
	RegisterBackend("http1Backend", func(name string, stage *Stage) Backend {
		b := new(HTTP1Backend)
		b.onCreate(name, stage)
		return b
	})
}

// HTTP1Backend
type HTTP1Backend struct {
	// Parent
	webBackend_[*http1Node]
	// States
}

func (b *HTTP1Backend) onCreate(name string, stage *Stage) {
	b.webBackend_.OnCreate(name, stage)
}

func (b *HTTP1Backend) OnConfigure() {
	b.webBackend_.OnConfigure()

	// sub components
	b.ConfigureNodes()
}
func (b *HTTP1Backend) OnPrepare() {
	b.webBackend_.OnPrepare()

	// sub components
	b.PrepareNodes()
}

func (b *HTTP1Backend) CreateNode(name string) Node {
	node := new(http1Node)
	node.onCreate(name, b)
	b.AddNode(node)
	return node
}

func (b *HTTP1Backend) FetchStream() (stream, error) {
	node := b.nodes[b.nextIndex()]
	return node.fetchStream()
}
func (b *HTTP1Backend) StoreStream(stream stream) {
	stream1 := stream.(*backend1Stream)
	stream1.conn.node.storeStream(stream1)
}

// http1Node is a node in HTTP1Backend.
type http1Node struct {
	// Parent
	webNode_
	// Assocs
	backend *HTTP1Backend
	// States
	connPool struct {
		sync.Mutex
		head *backend1Conn
		tail *backend1Conn
		qnty int
	}
}

func (n *http1Node) onCreate(name string, backend *HTTP1Backend) {
	n.webNode_.OnCreate(name)
	n.backend = backend
}

func (n *http1Node) OnConfigure() {
	n.webNode_.OnConfigure()
	if n.tlsMode {
		n.tlsConfig.InsecureSkipVerify = true
		n.tlsConfig.NextProtos = []string{"http/1.1"}
	}
}
func (n *http1Node) OnPrepare() {
	n.webNode_.OnPrepare()
}

func (n *http1Node) Maintain() { // runner
	n.LoopRun(time.Second, func(now time.Time) {
		// TODO: health check, markDown, markUp()
	})
	n.markDown()
	if size := n.closeFree(); size > 0 {
		n.DecSubs(size) // conns
	}
	n.WaitSubs() // conns. TODO: max timeout?
	if DebugLevel() >= 2 {
		Printf("http1Node=%s done\n", n.name)
	}
	n.backend.DecSub() // node
}

func (n *http1Node) fetchStream() (*backend1Stream, error) {
	conn := n.pullConn()
	down := n.isDown()
	if conn != nil {
		if conn.isAlive() && !conn.runOut() && !down {
			return conn.fetchStream()
		}
		conn.Close()
		n.DecSub() // conn
	}
	if down {
		return nil, errNodeDown
	}
	var err error
	if n.IsUDS() {
		conn, err = n._dialUDS()
	} else if n.IsTLS() {
		conn, err = n._dialTLS()
	} else {
		conn, err = n._dialTCP()
	}
	if err != nil {
		return nil, errNodeDown
	}
	n.IncSub() // conn
	return conn.fetchStream()
}
func (n *http1Node) storeStream(stream *backend1Stream) {
	conn := stream.conn
	conn.storeStream(stream)

	if conn.isBroken() || n.isDown() || !conn.isAlive() || !conn.persistent {
		conn.Close()
		n.DecSub() // conn
		if DebugLevel() >= 2 {
			Printf("Backend1Conn[node=%s id=%d] closed\n", conn.node.Name(), conn.id)
		}
	} else {
		n.pushConn(conn)
		if DebugLevel() >= 2 {
			Printf("Backend1Conn[node=%s id=%d] pushed\n", conn.node.Name(), conn.id)
		}
	}
}

func (n *http1Node) _dialUDS() (*backend1Conn, error) {
	// TODO: dynamic address names?
	netConn, err := net.DialTimeout("unix", n.address, n.backend.DialTimeout())
	if err != nil {
		n.markDown()
		return nil, err
	}
	if DebugLevel() >= 2 {
		Printf("http1Node=%s dial %s OK!\n", n.name, n.address)
	}
	connID := n.backend.nextConnID()
	rawConn, err := netConn.(*net.UnixConn).SyscallConn()
	if err != nil {
		netConn.Close()
		return nil, err
	}
	return getBackend1Conn(connID, n, netConn, rawConn), nil
}
func (n *http1Node) _dialTLS() (*backend1Conn, error) {
	// TODO: dynamic address names?
	netConn, err := net.DialTimeout("tcp", n.address, n.backend.DialTimeout())
	if err != nil {
		// TODO: handle ephemeral port exhaustion
		n.markDown()
		return nil, err
	}
	if DebugLevel() >= 2 {
		Printf("http1Node=%s dial %s OK!\n", n.name, n.address)
	}
	connID := n.backend.nextConnID()
	tlsConn := tls.Client(netConn, n.tlsConfig)
	if err := tlsConn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		tlsConn.Close()
		return nil, err
	}
	if err := tlsConn.Handshake(); err != nil {
		tlsConn.Close()
		return nil, err
	}
	return getBackend1Conn(connID, n, tlsConn, nil), nil
}
func (n *http1Node) _dialTCP() (*backend1Conn, error) {
	// TODO: dynamic address names?
	netConn, err := net.DialTimeout("tcp", n.address, n.backend.DialTimeout())
	if err != nil {
		// TODO: handle ephemeral port exhaustion
		n.markDown()
		return nil, err
	}
	if DebugLevel() >= 2 {
		Printf("http1Node=%s dial %s OK!\n", n.name, n.address)
	}
	connID := n.backend.nextConnID()
	rawConn, err := netConn.(*net.TCPConn).SyscallConn()
	if err != nil {
		netConn.Close()
		return nil, err
	}
	return getBackend1Conn(connID, n, netConn, rawConn), nil
}

func (n *http1Node) pullConn() *backend1Conn {
	list := &n.connPool

	list.Lock()
	defer list.Unlock()

	if list.qnty == 0 {
		return nil
	}
	conn := list.head
	list.head = conn.next
	conn.next = nil
	list.qnty--

	return conn
}
func (n *http1Node) pushConn(conn *backend1Conn) {
	list := &n.connPool

	list.Lock()
	defer list.Unlock()

	if list.qnty == 0 {
		list.head = conn
		list.tail = conn
	} else { // >= 1
		list.tail.next = conn
		list.tail = conn
	}
	list.qnty++
}
func (n *http1Node) closeFree() int {
	list := &n.connPool

	list.Lock()
	defer list.Unlock()

	for conn := list.head; conn != nil; conn = conn.next {
		conn.Close()
	}
	qnty := list.qnty
	list.qnty = 0
	list.head, list.tail = nil, nil

	return qnty
}

// backend1Conn is the backend-side HTTP/1.x connection.
type backend1Conn struct {
	// Parent
	http1Conn_
	// Assocs
	next   *backend1Conn  // the linked-list
	stream backend1Stream // an http/1.x connection has exactly one stream
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	node       *http1Node // the node to which the connection belongs
	expireTime time.Time  // when the conn is considered expired
	persistent bool       // keep the connection after current stream? true by default
	// Conn states (zeros)
}

var poolBackend1Conn sync.Pool

func getBackend1Conn(id int64, node *http1Node, netConn net.Conn, rawConn syscall.RawConn) *backend1Conn {
	var backendConn *backend1Conn
	if x := poolBackend1Conn.Get(); x == nil {
		backendConn = new(backend1Conn)
		stream := &backendConn.stream
		stream.conn = backendConn
		req, resp := &stream.request, &stream.response
		req.stream = stream
		req.outMessage = req
		req.response = resp
		resp.stream = stream
		resp.inMessage = resp
	} else {
		backendConn = x.(*backend1Conn)
	}
	backendConn.onGet(id, node, netConn, rawConn)
	return backendConn
}
func putBackend1Conn(backendConn *backend1Conn) {
	backendConn.onPut()
	poolBackend1Conn.Put(backendConn)
}

func (c *backend1Conn) onGet(id int64, node *http1Node, netConn net.Conn, rawConn syscall.RawConn) {
	backend := node.backend
	c.http1Conn_.onGet(id, backend.Stage().ID(), node.IsUDS(), node.IsTLS(), netConn, rawConn, backend.ReadTimeout(), backend.WriteTimeout())

	c.node = node
	c.expireTime = time.Now().Add(backend.idleTimeout)
	c.persistent = true
}
func (c *backend1Conn) onPut() {
	c.expireTime = time.Time{}
	c.node = nil

	c.http1Conn_.onPut()
}

func (c *backend1Conn) isAlive() bool { return time.Now().Before(c.expireTime) }

func (c *backend1Conn) runOut() bool {
	return c.usedStreams.Add(1) > c.node.backend.MaxStreamsPerConn()
}
func (c *backend1Conn) fetchStream() (*backend1Stream, error) {
	stream := &c.stream
	stream.onUse()
	return stream, nil
}
func (c *backend1Conn) storeStream(stream *backend1Stream) {
	stream.onEnd()
}

func (c *backend1Conn) Close() error {
	netConn := c.netConn
	putBackend1Conn(c)
	return netConn.Close()
}

// backend1Stream is the backend-side HTTP/1.x stream.
type backend1Stream struct {
	// Parent
	http1Stream_[*backend1Conn]
	// Assocs
	request  backend1Request  // the backend-side http/1.x request
	response backend1Response // the backend-side http/1.x response
	socket   *backend1Socket  // the backend-side http/1.x webSocket
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non zeros)
	// Stream states (zeros)
}

func (s *backend1Stream) onUse() { // for non-zeros
	s.http1Stream_.onUse()

	s.request.onUse(Version1_1)
	s.response.onUse(Version1_1)
}
func (s *backend1Stream) onEnd() { // for zeros
	s.response.onEnd()
	s.request.onEnd()
	if s.socket != nil {
		s.socket.onEnd()
		s.socket = nil
	}

	s.http1Stream_.onEnd()
}

func (s *backend1Stream) Holder() webHolder { return s.conn.node.backend }

func (s *backend1Stream) Request() request   { return &s.request }
func (s *backend1Stream) Response() response { return &s.response }
func (s *backend1Stream) Socket() socket     { return nil } // TODO. See RFC 6455

// backend1Request is the backend-side HTTP/1.x request.
type backend1Request struct { // outgoing. needs building
	// Parent
	backendRequest_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *backend1Request) setMethodURI(method []byte, uri []byte, hasContent bool) bool { // METHOD uri HTTP/1.1\r\n
	controlSize := len(method) + 1 + len(uri) + 1 + len(bytesHTTP1_1) + len(bytesCRLF)
	if from, edge, ok := r._growFields(controlSize); ok {
		from += copy(r.fields[from:], method)
		r.fields[from] = ' '
		from++
		from += copy(r.fields[from:], uri)
		r.fields[from] = ' '
		from++
		from += copy(r.fields[from:], bytesHTTP1_1) // we always use HTTP/1.1
		r.fields[from] = '\r'
		r.fields[from+1] = '\n'
		if !hasContent {
			r.forbidContent = true
			r.forbidFraming = true
		}
		r.controlEdge = uint16(edge)
		return true
	} else {
		return false
	}
}
func (r *backend1Request) proxySetAuthority(hostname []byte, colonport []byte) bool {
	if r.stream.Conn().IsTLS() {
		if bytes.Equal(colonport, bytesColonport443) {
			colonport = nil
		}
	} else if bytes.Equal(colonport, bytesColonport80) {
		colonport = nil
	}
	headerSize := len(bytesHost) + len(bytesColonSpace) + len(hostname) + len(colonport) + len(bytesCRLF) // host: xxx\r\n
	if from, _, ok := r._growFields(headerSize); ok {
		from += copy(r.fields[from:], bytesHost)
		r.fields[from] = ':'
		r.fields[from+1] = ' '
		from += 2
		from += copy(r.fields[from:], hostname)
		from += copy(r.fields[from:], colonport)
		r._addCRLFHeader1(from)
		return true
	} else {
		return false
	}
}

func (r *backend1Request) addHeader(name []byte, value []byte) bool   { return r.addHeader1(name, value) }
func (r *backend1Request) header(name []byte) (value []byte, ok bool) { return r.header1(name) }
func (r *backend1Request) hasHeader(name []byte) bool                 { return r.hasHeader1(name) }
func (r *backend1Request) delHeader(name []byte) (deleted bool)       { return r.delHeader1(name) }
func (r *backend1Request) delHeaderAt(i uint8)                        { r.delHeaderAt1(i) }

func (r *backend1Request) AddCookie(name string, value string) bool { // cookie: foo=bar; xyz=baz
	// TODO. need some space to place the cookie. use stream.unsafeMake()?
	return false
}
func (r *backend1Request) proxyCopyCookies(foreReq Request) bool { // merge all cookies into one "cookie" header
	headerSize := len(bytesCookie) + len(bytesColonSpace) // `cookie: `
	foreReq.forCookies(func(cookie *pair, name []byte, value []byte) bool {
		headerSize += len(name) + 1 + len(value) + 2 // `name=value; `
		return true
	})
	if from, _, ok := r.growHeader(headerSize); ok {
		from += copy(r.fields[from:], bytesCookie)
		r.fields[from] = ':'
		r.fields[from+1] = ' '
		from += 2
		foreReq.forCookies(func(cookie *pair, name []byte, value []byte) bool {
			from += copy(r.fields[from:], name)
			r.fields[from] = '='
			from++
			from += copy(r.fields[from:], value)
			r.fields[from] = ';'
			r.fields[from+1] = ' '
			from += 2
			return true
		})
		r.fields[from-2] = '\r'
		r.fields[from-1] = '\n'
		return true
	} else {
		return false
	}
}

func (r *backend1Request) sendChain() error { return r.sendChain1() }

func (r *backend1Request) echoHeaders() error { return r.writeHeaders1() }
func (r *backend1Request) echoChain() error   { return r.echoChain1(true) } // we always use HTTP/1.1 chunked

func (r *backend1Request) addTrailer(name []byte, value []byte) bool {
	return r.addTrailer1(name, value)
}
func (r *backend1Request) trailer(name []byte) (value []byte, ok bool) { return r.trailer1(name) }

func (r *backend1Request) proxyPassHeaders() error       { return r.writeHeaders1() }
func (r *backend1Request) proxyPassBytes(p []byte) error { return r.proxyPassBytes1(p) }

func (r *backend1Request) finalizeHeaders() { // add at most 256 bytes
	// if-modified-since: Sun, 06 Nov 1994 08:49:37 GMT\r\n
	if r.unixTimes.ifModifiedSince >= 0 {
		r.fieldsEdge += uint16(clockWriteHTTPDate1(r.fields[r.fieldsEdge:], bytesIfModifiedSince, r.unixTimes.ifModifiedSince))
	}
	// if-unmodified-since: Sun, 06 Nov 1994 08:49:37 GMT\r\n
	if r.unixTimes.ifUnmodifiedSince >= 0 {
		r.fieldsEdge += uint16(clockWriteHTTPDate1(r.fields[r.fieldsEdge:], bytesIfUnmodifiedSince, r.unixTimes.ifUnmodifiedSince))
	}
	if r.contentSize != -1 { // with content
		if !r.forbidFraming {
			if r.isVague() { // transfer-encoding: chunked\r\n
				r.fieldsEdge += uint16(copy(r.fields[r.fieldsEdge:], http1BytesTransferChunked))
			} else { // content-length: >=0\r\n
				sizeBuffer := r.stream.buffer256() // enough for content-length
				n := i64ToDec(r.contentSize, sizeBuffer)
				r._addFixedHeader1(bytesContentLength, sizeBuffer[:n])
			}
		}
		// content-type: application/octet-stream\r\n
		if r.iContentType == 0 {
			r.fieldsEdge += uint16(copy(r.fields[r.fieldsEdge:], http1BytesContentTypeStream))
		}
	}
	// connection: keep-alive\r\n
	r.fieldsEdge += uint16(copy(r.fields[r.fieldsEdge:], http1BytesConnectionKeepAlive))
}
func (r *backend1Request) finalizeVague() error { return r.finalizeVague1() }

func (r *backend1Request) addedHeaders() []byte { return r.fields[r.controlEdge:r.fieldsEdge] }
func (r *backend1Request) fixedHeaders() []byte { return http1BytesFixedRequestHeaders }

// backend1Response is the backend-side HTTP/1.x response.
type backend1Response struct { // incoming. needs parsing
	// Parent
	backendResponse_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *backend1Response) recvHead() { // status-line + headers
	// The entire response head must be received within one timeout
	if err := r._beforeRead(&r.recvTime); err != nil {
		r.headResult = -1
		return
	}
	if !r.growHead1() { // r.input must be empty because we don't use pipelining in requests.
		// r.headResult is set.
		return
	}
	if !r._recvStatusLine() || !r.recvHeaders1() || !r.examineHead() {
		// r.headResult is set.
		return
	}
	r.cleanInput()
	if DebugLevel() >= 2 {
		Printf("[backend1Stream=%d]<======= [%s]\n", r.stream.Conn().ID(), r.input[r.head.from:r.head.edge])
	}
}
func (r *backend1Response) _recvStatusLine() bool { // status-line = HTTP-version SP status-code SP [ reason-phrase ] CRLF
	// HTTP-version = HTTP-name "/" DIGIT "." DIGIT
	// HTTP-name = %x48.54.54.50 ; "HTTP", case-sensitive
	if have := r.inputEdge - r.elemFore; have >= 9 {
		// r.elemFore -> ' '
		// r.inputEdge -> after ' ' or more
		r.elemFore += 8
	} else { // have < 9, but len("HTTP/1.X ") = 9.
		// r.elemFore at 'H' -> ' '
		// r.inputEdge at "TTP/1.X " -> after ' '
		r.elemFore = r.inputEdge - 1
		for i, n := int32(0), 9-have; i < n; i++ {
			if r.elemFore++; r.elemFore == r.inputEdge && !r.growHead1() {
				return false
			}
		}
	}
	if !bytes.Equal(r.input[r.elemBack:r.elemFore], bytesHTTP1_1) { // for HTTP/1.x, only HTTP/1.1 is supported in backend side
		r.headResult = StatusHTTPVersionNotSupported
		return false
	}

	// Skip SP
	if r.input[r.elemFore] != ' ' {
		goto invalid
	}
	if r.elemFore++; r.elemFore == r.inputEdge && !r.growHead1() {
		return false
	}

	// status-code = 3DIGIT
	if b := r.input[r.elemFore]; b >= '1' && b <= '9' {
		r.status = int16(b-'0') * 100
	} else {
		goto invalid
	}
	if r.elemFore++; r.elemFore == r.inputEdge && !r.growHead1() {
		return false
	}
	if b := r.input[r.elemFore]; b >= '0' && b <= '9' {
		r.status += int16(b-'0') * 10
	} else {
		goto invalid
	}
	if r.elemFore++; r.elemFore == r.inputEdge && !r.growHead1() {
		return false
	}
	if b := r.input[r.elemFore]; b >= '0' && b <= '9' {
		r.status += int16(b - '0')
	} else {
		goto invalid
	}
	if r.elemFore++; r.elemFore == r.inputEdge && !r.growHead1() {
		return false
	}

	// Skip SP
	if r.input[r.elemFore] != ' ' {
		goto invalid
	}
	if r.elemFore++; r.elemFore == r.inputEdge && !r.growHead1() {
		return false
	}

	// reason-phrase = 1*( HTAB / SP / VCHAR / obs-text )
	for {
		if b := r.input[r.elemFore]; b == '\n' {
			break
		}
		if r.elemFore++; r.elemFore == r.inputEdge && !r.growHead1() {
			return false
		}
	}
	r.receiving = httpSectionHeaders
	// Skip '\n'
	if r.elemFore++; r.elemFore == r.inputEdge && !r.growHead1() {
		return false
	}
	return true
invalid:
	r.headResult, r.failReason = StatusBadRequest, "invalid character in control"
	return false
}
func (r *backend1Response) cleanInput() {
	// r.elemFore is at the beginning of content (if exists) or next response (if exists and is pipelined).
	if r.contentSize == -1 { // no content
		r.contentReceived = true
		if r.elemFore < r.inputEdge { // still has data
			// RFC 9112 (section 6.3):
			// If the final response to the last request on a connection has been completely received
			// and there remains additional data to read, a user agent MAY discard the remaining data
			// or attempt to determine if that data belongs as part of the prior message body, which
			// might be the case if the prior message's Content-Length value is incorrect. A client
			// MUST NOT process, cache, or forward such extra data as a separate response, since such
			// behavior would be vulnerable to cache poisoning.

			// TODO: log? possible response splitting
		}
		return
	}
	// content exists (sized or vague)
	r.imme.set(r.elemFore, r.inputEdge)
	if r.contentSize >= 0 { // sized mode
		if immeSize := int64(r.imme.size()); immeSize >= r.contentSize {
			r.contentReceived = true
			if immeSize > r.contentSize { // still has data
				// TODO: log? possible response splitting
			}
			r.receivedSize = r.contentSize
			r.contentText = r.input[r.elemFore : r.elemFore+int32(r.contentSize)] // exact.
			r.contentTextKind = httpContentTextInput
		}
	} else { // vague mode
		// We don't know the size of vague content. Let chunked receivers to decide & clean r.input.
	}
}

func (r *backend1Response) readContent() (p []byte, err error) { return r.readContent1() }

// backend1Socket is the backend-side HTTP/1.x webSocket.
type backend1Socket struct { // incoming and outgoing
	// Parent
	backendSocket_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

var poolBackend1Socket sync.Pool

func getBackend1Socket(stream *backend1Stream) *backend1Socket {
	// TODO
	return nil
}
func putBackend1Socket(socket *backend1Socket) {
	// TODO
}

func (s *backend1Socket) onUse() {
	s.backendSocket_.onUse()
}
func (s *backend1Socket) onEnd() {
	s.backendSocket_.onEnd()
}

//////////////////////////////////////// HTTP/1.x in/out implementation ////////////////////////////////////////

// http1Conn
type http1Conn interface {
	// Imports
	webConn
	// Methods
	setReadDeadline() error
	setWriteDeadline() error
	read(p []byte) (int, error)
	readFull(p []byte) (int, error)
	write(p []byte) (int, error)
	writev(vector *net.Buffers) (int64, error)
}

// http1Conn_
type http1Conn_ struct {
	// Parent
	webConn_
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	netConn net.Conn        // *net.TCPConn, *tls.Conn, *net.UnixConn
	rawConn syscall.RawConn // for syscall, only usable when netConn is TCP/UDS
	// Conn states (zeros)
}

func (c *http1Conn_) onGet(id int64, stageID int32, udsMode bool, tlsMode bool, netConn net.Conn, rawConn syscall.RawConn, readTimeout time.Duration, writeTimeout time.Duration) {
	c.webConn_.onGet(id, stageID, udsMode, tlsMode, readTimeout, writeTimeout)

	c.netConn = netConn
	c.rawConn = rawConn
}
func (c *http1Conn_) onPut() {
	c.netConn = nil
	c.rawConn = nil

	c.webConn_.onPut()
}

func (c *http1Conn_) remoteAddr() net.Addr { return c.netConn.RemoteAddr() }

func (c *http1Conn_) setReadDeadline() error {
	deadline := time.Now().Add(c.readTimeout)
	if deadline.Sub(c.lastRead) >= time.Second {
		if err := c.netConn.SetReadDeadline(deadline); err != nil {
			return err
		}
		c.lastRead = deadline
	}
	return nil
}
func (c *http1Conn_) setWriteDeadline() error {
	deadline := time.Now().Add(c.writeTimeout)
	if deadline.Sub(c.lastWrite) >= time.Second {
		if err := c.netConn.SetWriteDeadline(deadline); err != nil {
			return err
		}
		c.lastWrite = deadline
	}
	return nil
}

func (c *http1Conn_) read(p []byte) (int, error)                { return c.netConn.Read(p) }
func (c *http1Conn_) readFull(p []byte) (int, error)            { return io.ReadFull(c.netConn, p) }
func (c *http1Conn_) write(p []byte) (int, error)               { return c.netConn.Write(p) }
func (c *http1Conn_) writev(vector *net.Buffers) (int64, error) { return vector.WriteTo(c.netConn) }

// http1Stream
type http1Stream interface {
	// Imports
	webStream
	// Methods
}

// http1Stream_
type http1Stream_[C http1Conn] struct {
	// Parent
	webStream_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non zeros)
	conn C // the http/1.x connection
	// Stream states (zeros)
}

func (s *http1Stream_[C]) onUse() {
	s.webStream_.onUse()
}
func (s *http1Stream_[C]) onEnd() {
	s.webStream_.onEnd()
}

func (s *http1Stream_[C]) Conn() webConn        { return s.conn }
func (s *http1Stream_[C]) remoteAddr() net.Addr { return s.conn.remoteAddr() }

func (s *http1Stream_[C]) markBroken()    { s.conn.markBroken() }
func (s *http1Stream_[C]) isBroken() bool { return s.conn.isBroken() }

func (s *http1Stream_[C]) setReadDeadline() error  { return s.conn.setReadDeadline() }
func (s *http1Stream_[C]) setWriteDeadline() error { return s.conn.setWriteDeadline() }

func (s *http1Stream_[C]) read(p []byte) (int, error)     { return s.conn.read(p) }
func (s *http1Stream_[C]) readFull(p []byte) (int, error) { return s.conn.readFull(p) }
func (s *http1Stream_[C]) write(p []byte) (int, error)    { return s.conn.write(p) }
func (s *http1Stream_[C]) writev(vector *net.Buffers) (int64, error) {
	return s.conn.writev(vector)
}

// HTTP/1.x incoming

func (r *webIn_) growHead1() bool { // HTTP/1.x is not a binary protocol, we don't know how many bytes to grow, so just grow.
	// Is r.input full?
	if inputSize := int32(cap(r.input)); r.inputEdge == inputSize { // r.inputEdge reached end, so r.input is full
		if inputSize == _16K { // max r.input size is 16K, we cannot use a larger input anymore
			if r.receiving == httpSectionControl {
				r.headResult = StatusURITooLong
			} else { // httpSectionHeaders
				r.headResult = StatusRequestHeaderFieldsTooLarge
			}
			return false
		}
		// r.input size < 16K. We switch to a larger input (stock -> 4K -> 16K)
		stockSize := int32(cap(r.stockInput))
		var input []byte
		if inputSize == stockSize {
			input = Get4K()
		} else { // 4K
			input = Get16K()
		}
		copy(input, r.input) // copy all
		if inputSize != stockSize {
			PutNK(r.input)
		}
		r.input = input // a larger input is now used
	}
	// r.input is not full.
	if n, err := r.stream.read(r.input[r.inputEdge:]); err == nil {
		r.inputEdge += int32(n) // we might have only read 1 byte.
		return true
	} else if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		r.headResult = StatusRequestTimeout
	} else { // i/o error or unexpected EOF
		r.headResult = -1
	}
	return false
}
func (r *webIn_) recvHeaders1() bool { // *( field-name ":" OWS field-value OWS CRLF ) CRLF
	r.headers.from = uint8(len(r.primes))
	r.headers.edge = r.headers.from
	header := &r.mainPair
	header.zero()
	header.kind = pairHeader
	header.place = placeInput // all received headers are in r.input
	// r.elemFore is at headers (if any) or end of headers (if none).
	for { // each header
		// End of headers?
		if b := r.input[r.elemFore]; b == '\r' {
			// Skip '\r'
			if r.elemFore++; r.elemFore == r.inputEdge && !r.growHead1() {
				return false
			}
			if r.input[r.elemFore] != '\n' {
				r.headResult, r.failReason = StatusBadRequest, "bad end of headers"
				return false
			}
			break
		} else if b == '\n' {
			break
		}

		// header-field = field-name ":" OWS field-value OWS

		// field-name = token
		// token = 1*tchar

		r.elemBack = r.elemFore // now r.elemBack is at header-field
		for {
			b := r.input[r.elemFore]
			if t := httpTchar[b]; t == 1 {
				// Fast path, do nothing
			} else if t == 2 { // A-Z
				b += 0x20 // to lower
				r.input[r.elemFore] = b
			} else if t == 3 { // '_'
				header.setUnderscore()
			} else if b == ':' {
				break
			} else {
				r.headResult, r.failReason = StatusBadRequest, "header name contains bad character"
				return false
			}
			header.nameHash += uint16(b)
			if r.elemFore++; r.elemFore == r.inputEdge && !r.growHead1() {
				return false
			}
		}
		if nameSize := r.elemFore - r.elemBack; nameSize > 0 && nameSize <= 255 {
			header.nameFrom, header.nameSize = r.elemBack, uint8(nameSize)
		} else {
			r.headResult, r.failReason = StatusBadRequest, "header name out of range"
			return false
		}
		// Skip ':'
		if r.elemFore++; r.elemFore == r.inputEdge && !r.growHead1() {
			return false
		}
		// Skip OWS before field-value (and OWS after field-value if it is empty)
		for r.input[r.elemFore] == ' ' || r.input[r.elemFore] == '\t' {
			if r.elemFore++; r.elemFore == r.inputEdge && !r.growHead1() {
				return false
			}
		}
		// field-value   = *field-content
		// field-content = field-vchar [ 1*( %x20 / %x09 / field-vchar) field-vchar ]
		// field-vchar   = %x21-7E / %x80-FF
		// In other words, a string of octets is a field-value if and only if:
		// - it is *( %x21-7E / %x80-FF / %x20 / %x09)
		// - if it is not empty, it starts and ends with field-vchar
		r.elemBack = r.elemFore // now r.elemBack is at field-value (if not empty) or EOL (if field-value is empty)
		for {
			if b := r.input[r.elemFore]; (b >= 0x20 && b != 0x7F) || b == 0x09 {
				if r.elemFore++; r.elemFore == r.inputEdge && !r.growHead1() {
					return false
				}
			} else if b == '\r' {
				// Skip '\r'
				if r.elemFore++; r.elemFore == r.inputEdge && !r.growHead1() {
					return false
				}
				if r.input[r.elemFore] != '\n' {
					r.headResult, r.failReason = StatusBadRequest, "header value contains bad eol"
					return false
				}
				break
			} else if b == '\n' {
				break
			} else {
				r.headResult, r.failReason = StatusBadRequest, "header value contains bad character"
				return false
			}
		}
		// r.elemFore is at '\n'
		fore := r.elemFore
		if r.input[fore-1] == '\r' {
			fore--
		}
		if fore > r.elemBack { // field-value is not empty. now trim OWS after field-value
			for r.input[fore-1] == ' ' || r.input[fore-1] == '\t' {
				fore--
			}
		}
		header.value.set(r.elemBack, fore)

		// Header is received in general algorithm. Now add it
		if !r.addHeader(header) {
			// r.headResult is set.
			return false
		}

		// Header is successfully received. Skip '\n'
		if r.elemFore++; r.elemFore == r.inputEdge && !r.growHead1() {
			return false
		}
		// r.elemFore is now at the next header or end of headers.
		header.nameHash, header.flags = 0, 0 // reset for next header
	}
	r.receiving = httpSectionContent
	// Skip end of headers
	r.elemFore++
	// Now the head is received, and r.elemFore is at the beginning of content (if exists) or next message (if exists and is pipelined).
	r.head.set(0, r.elemFore)

	return true
}

func (r *webIn_) readContent1() (p []byte, err error) {
	if r.contentSize >= 0 { // sized
		return r._readSizedContent1()
	} else { // vague. must be -2. -1 (no content) is excluded priorly
		return r._readVagueContent1()
	}
}
func (r *webIn_) _readSizedContent1() (p []byte, err error) {
	if r.receivedSize == r.contentSize { // content is entirely received
		if r.bodyWindow == nil { // body window is not used. this means content is immediate
			return r.contentText[:r.receivedSize], io.EOF
		} else { // r.bodyWindow was used.
			PutNK(r.bodyWindow)
			r.bodyWindow = nil
			return nil, io.EOF
		}
	}
	// Need more content text.
	if r.bodyWindow == nil {
		r.bodyWindow = Get16K() // will be freed on ends. must be >= 16K so r.imme can fit
	}
	if r.imme.notEmpty() {
		immeSize := copy(r.bodyWindow, r.input[r.imme.from:r.imme.edge]) // r.input is not larger than r.bodyWindow
		r.receivedSize = int64(immeSize)
		r.imme.zero()
		return r.bodyWindow[0:immeSize], nil
	}
	if err = r._beforeRead(&r.bodyTime); err != nil {
		return nil, err
	}
	readSize := int64(cap(r.bodyWindow))
	if sizeLeft := r.contentSize - r.receivedSize; sizeLeft < readSize {
		readSize = sizeLeft
	}
	size, err := r.stream.readFull(r.bodyWindow[:readSize])
	if err == nil {
		if !r._tooSlow() {
			r.receivedSize += int64(size)
			return r.bodyWindow[:size], nil
		}
		err = webInTooSlow
	}
	return nil, err
}
func (r *webIn_) _readVagueContent1() (p []byte, err error) {
	if r.bodyWindow == nil {
		r.bodyWindow = Get16K() // will be freed on ends. 16K is a tradeoff between performance and memory consumption, and can fit r.imme and trailers
	}
	if r.imme.notEmpty() {
		r.chunkEdge = int32(copy(r.bodyWindow, r.input[r.imme.from:r.imme.edge])) // r.input is not larger than r.bodyWindow
		r.imme.zero()
	}
	if r.chunkEdge == 0 && !r.growChunked1() { // r.bodyWindow is empty. must fill
		goto badRead
	}
	switch r.chunkSize { // size left in receiving current chunk
	case -2: // got chunk-data. needs CRLF or LF
		if r.bodyWindow[r.chunkFore] == '\r' {
			if r.chunkFore++; r.chunkFore == r.chunkEdge && !r.growChunked1() {
				goto badRead
			}
		}
		fallthrough
	case -1: // got chunk-data CR. needs LF
		if r.bodyWindow[r.chunkFore] != '\n' {
			goto badRead
		}
		// Skip '\n'
		if r.chunkFore++; r.chunkFore == r.chunkEdge && !r.growChunked1() {
			goto badRead
		}
		fallthrough
	case 0: // start a new chunk = chunk-size [chunk-ext] CRLF chunk-data CRLF
		r.chunkBack = r.chunkFore // now r.bodyWindow is used for receiving: chunk-size [chunk-ext] CRLF
		chunkSize := int64(0)
		for { // chunk-size = 1*HEXDIG
			b := r.bodyWindow[r.chunkFore]
			if b >= '0' && b <= '9' {
				b = b - '0'
			} else if b >= 'a' && b <= 'f' {
				b = b - 'a' + 10
			} else if b >= 'A' && b <= 'F' {
				b = b - 'A' + 10
			} else {
				break
			}
			chunkSize <<= 4
			chunkSize += int64(b)
			if r.chunkFore++; r.chunkFore-r.chunkBack >= 16 || (r.chunkFore == r.chunkEdge && !r.growChunked1()) {
				goto badRead
			}
		}
		if chunkSize < 0 { // bad chunk size.
			goto badRead
		}
		if b := r.bodyWindow[r.chunkFore]; b == ';' { // ignore chunk-ext = *( ";" chunk-ext-name [ "=" chunk-ext-val ] )
			for r.bodyWindow[r.chunkFore] != '\n' {
				if r.chunkFore++; r.chunkFore == r.chunkEdge && !r.growChunked1() {
					goto badRead
				}
			}
		} else if b == '\r' {
			// Skip '\r'
			if r.chunkFore++; r.chunkFore == r.chunkEdge && !r.growChunked1() {
				goto badRead
			}
		}
		// Must be LF
		if r.bodyWindow[r.chunkFore] != '\n' {
			goto badRead
		}
		// Check target size
		if targetSize := r.receivedSize + chunkSize; targetSize >= 0 && targetSize <= r.maxContentSize {
			r.chunkSize = chunkSize
		} else { // invalid target size.
			// TODO: log error?
			goto badRead
		}
		// Skip '\n' at the end of: chunk-size [chunk-ext] CRLF
		if r.chunkFore++; r.chunkFore == r.chunkEdge && !r.growChunked1() {
			goto badRead
		}
		// Last chunk?
		if r.chunkSize == 0 { // last-chunk = 1*("0") [chunk-ext] CRLF
			// last-chunk trailer-section CRLF
			if r.bodyWindow[r.chunkFore] == '\r' {
				// Skip '\r'
				if r.chunkFore++; r.chunkFore == r.chunkEdge && !r.growChunked1() {
					goto badRead
				}
				if r.bodyWindow[r.chunkFore] != '\n' {
					goto badRead
				}
			} else if r.bodyWindow[r.chunkFore] != '\n' { // must be trailer-section = *( field-line CRLF)
				r.receiving = httpSectionTrailers
				if !r.recvTrailers1() || !r.inMessage.examineTail() {
					goto badRead
				}
				// r.recvTrailers1() must ends with r.chunkFore being at the last '\n' after trailer-section.
			}
			// Skip the last '\n'
			r.chunkFore++ // now the whole vague content is received and r.chunkFore is immediately after the vague content.
			// Now we have found the end of current message, so determine r.inputNext and r.inputEdge.
			if r.chunkFore < r.chunkEdge { // still has data, stream is pipelined
				r.overChunked = true                                // so r.bodyWindow will be used as r.input on stream ends
				r.inputNext, r.inputEdge = r.chunkFore, r.chunkEdge // mark the next message
			} else { // no data anymore, stream is not pipelined
				r.inputNext, r.inputEdge = 0, 0 // reset input
				PutNK(r.bodyWindow)
				r.bodyWindow = nil
			}
			return nil, io.EOF
		}
		// Not last chunk, now r.chunkFore is at the beginning of: chunk-data CRLF
		fallthrough
	default: // r.chunkSize > 0, we are receiving: chunk-data CRLF
		r.chunkBack = 0 // so growChunked1() works correctly
		var data span   // the chunk data we are receiving
		data.from = r.chunkFore
		if haveSize := int64(r.chunkEdge - r.chunkFore); haveSize <= r.chunkSize { // 1 <= haveSize <= r.chunkSize. chunk-data can be taken entirely
			r.receivedSize += haveSize
			data.edge = r.chunkEdge
			if haveSize == r.chunkSize { // exact chunk-data
				r.chunkSize = -2 // got chunk-data, needs CRLF or LF
			} else { // haveSize < r.chunkSize, not enough data.
				r.chunkSize -= haveSize
			}
			r.chunkFore, r.chunkEdge = 0, 0 // all data taken
		} else { // haveSize > r.chunkSize, more than chunk-data
			r.receivedSize += r.chunkSize
			data.edge = r.chunkFore + int32(r.chunkSize)
			if sizeLeft := r.chunkEdge - data.edge; sizeLeft == 1 { // chunk-data ?
				if b := r.bodyWindow[data.edge]; b == '\r' { // exact chunk-data CR
					r.chunkSize = -1 // got chunk-data CR, needs LF
				} else if b == '\n' { // exact chunk-data LF
					r.chunkSize = 0
				} else { // chunk-data X
					goto badRead
				}
				r.chunkFore, r.chunkEdge = 0, 0 // all data taken
			} else if r.bodyWindow[data.edge] == '\r' && r.bodyWindow[data.edge+1] == '\n' { // chunk-data CRLF..
				r.chunkSize = 0
				if sizeLeft == 2 { // exact chunk-data CRLF
					r.chunkFore, r.chunkEdge = 0, 0 // all data taken
				} else { // > 2, chunk-data CRLF X
					r.chunkFore = data.edge + 2
				}
			} else if r.bodyWindow[data.edge] == '\n' { // >=2, chunk-data LF X
				r.chunkSize = 0
				r.chunkFore = data.edge + 1
			} else { // >=2, chunk-data XX
				goto badRead
			}
		}
		return r.bodyWindow[data.from:data.edge], nil
	}
badRead:
	return nil, webInBadChunk
}

func (r *webIn_) recvTrailers1() bool { // trailer-section = *( field-line CRLF)
	copy(r.bodyWindow, r.bodyWindow[r.chunkFore:r.chunkEdge]) // slide to start, we need a clean r.bodyWindow
	r.chunkEdge -= r.chunkFore
	r.chunkBack, r.chunkFore = 0, 0 // setting r.chunkBack = 0 means r.bodyWindow will not slide, so the whole trailers must fit in r.bodyWindow.
	r.elemBack, r.elemFore = 0, 0   // for parsing trailer fields

	r.trailers.from = uint8(len(r.primes))
	r.trailers.edge = r.trailers.from
	trailer := &r.mainPair
	trailer.zero()
	trailer.kind = pairTrailer
	trailer.place = placeArray // all received trailers are placed in r.array
	for {
		if b := r.bodyWindow[r.elemFore]; b == '\r' {
			// Skip '\r'
			if r.elemFore++; r.elemFore == r.chunkEdge && !r.growChunked1() {
				return false
			}
			if r.bodyWindow[r.elemFore] != '\n' {
				return false
			}
			break
		} else if b == '\n' {
			break
		}

		r.elemBack = r.elemFore // for field-name
		for {
			b := r.bodyWindow[r.elemFore]
			if t := httpTchar[b]; t == 1 {
				// Fast path, do nothing
			} else if t == 2 { // A-Z
				b += 0x20 // to lower
				r.bodyWindow[r.elemFore] = b
			} else if t == 3 { // '_'
				trailer.setUnderscore()
			} else if b == ':' {
				break
			} else {
				return false
			}
			trailer.nameHash += uint16(b)
			if r.elemFore++; r.elemFore == r.chunkEdge && !r.growChunked1() {
				return false
			}
		}
		if nameSize := r.elemFore - r.elemBack; nameSize > 0 && nameSize <= 255 {
			trailer.nameFrom, trailer.nameSize = r.elemBack, uint8(nameSize)
		} else {
			return false
		}
		// Skip ':'
		if r.elemFore++; r.elemFore == r.chunkEdge && !r.growChunked1() {
			return false
		}
		// Skip OWS before field-value (and OWS after field-value if it is empty)
		for r.bodyWindow[r.elemFore] == ' ' || r.bodyWindow[r.elemFore] == '\t' {
			if r.elemFore++; r.elemFore == r.chunkEdge && !r.growChunked1() {
				return false
			}
		}
		r.elemBack = r.elemFore // for field-value or EOL
		for {
			if b := r.bodyWindow[r.elemFore]; (b >= 0x20 && b != 0x7F) || b == 0x09 {
				if r.elemFore++; r.elemFore == r.chunkEdge && !r.growChunked1() {
					return false
				}
			} else if b == '\r' {
				// Skip '\r'
				if r.elemFore++; r.elemFore == r.chunkEdge && !r.growChunked1() {
					return false
				}
				if r.bodyWindow[r.elemFore] != '\n' {
					return false
				}
				break
			} else if b == '\n' {
				break
			} else {
				return false
			}
		}
		// r.elemFore is at '\n'
		fore := r.elemFore
		if r.bodyWindow[fore-1] == '\r' {
			fore--
		}
		if fore > r.elemBack { // field-value is not empty. now trim OWS after field-value
			for r.bodyWindow[fore-1] == ' ' || r.bodyWindow[fore-1] == '\t' {
				fore--
			}
		}
		trailer.value.set(r.elemBack, fore)

		// Copy trailer data to r.array
		fore = r.arrayEdge
		if !r.arrayCopy(trailer.nameAt(r.bodyWindow)) {
			return false
		}
		trailer.nameFrom = fore
		fore = r.arrayEdge
		if !r.arrayCopy(trailer.valueAt(r.bodyWindow)) {
			return false
		}
		trailer.value.set(fore, r.arrayEdge)

		// Trailer is received in general algorithm. Now add it
		if !r.addTrailer(trailer) {
			return false
		}

		// Trailer is successfully received. Skip '\n'
		if r.elemFore++; r.elemFore == r.chunkEdge && !r.growChunked1() {
			return false
		}
		// r.elemFore is now at the next trailer or end of trailers.
		trailer.nameHash, trailer.flags = 0, 0 // reset for next trailer
	}
	r.chunkFore = r.elemFore // r.chunkFore must ends at the last '\n'
	return true
}
func (r *webIn_) growChunked1() bool { // HTTP/1.x is not a binary protocol, we don't know how many bytes to grow, so just grow.
	if r.chunkEdge == int32(cap(r.bodyWindow)) && r.chunkBack == 0 { // r.bodyWindow is full and we can't slide
		return false // element is too large
	}
	if r.chunkBack > 0 { // has previously used data, but now useless. slide to start so we can read more
		copy(r.bodyWindow, r.bodyWindow[r.chunkBack:r.chunkEdge])
		r.chunkEdge -= r.chunkBack
		r.chunkFore -= r.chunkBack
		r.chunkBack = 0
	}
	err := r._beforeRead(&r.bodyTime)
	if err == nil {
		n, e := r.stream.read(r.bodyWindow[r.chunkEdge:])
		r.chunkEdge += int32(n)
		if e == nil {
			if !r._tooSlow() {
				return true
			}
			e = webInTooSlow
		}
		err = e // including io.EOF which is unexpected here
	}
	// err != nil. TODO: log err
	return false
}

// HTTP/1.x outgoing

func (r *webOut_) addHeader1(name []byte, value []byte) bool {
	if len(name) == 0 {
		return false
	}
	headerSize := len(name) + len(bytesColonSpace) + len(value) + len(bytesCRLF) // name: value\r\n
	if from, _, ok := r.growHeader(headerSize); ok {
		from += copy(r.fields[from:], name)
		r.fields[from] = ':'
		r.fields[from+1] = ' '
		from += 2
		from += copy(r.fields[from:], value)
		r._addCRLFHeader1(from)
		return true
	} else {
		return false
	}
}
func (r *webOut_) header1(name []byte) (value []byte, ok bool) {
	if r.nHeaders > 1 && len(name) > 0 {
		from := uint16(0)
		for i := uint8(1); i < r.nHeaders; i++ {
			edge := r.edges[i]
			header := r.fields[from:edge]
			if p := bytes.IndexByte(header, ':'); p != -1 && bytes.Equal(header[0:p], name) {
				return header[p+len(bytesColonSpace) : len(header)-len(bytesCRLF)], true
			}
			from = edge
		}
	}
	return
}
func (r *webOut_) hasHeader1(name []byte) bool {
	if r.nHeaders > 1 && len(name) > 0 {
		from := uint16(0)
		for i := uint8(1); i < r.nHeaders; i++ {
			edge := r.edges[i]
			header := r.fields[from:edge]
			if p := bytes.IndexByte(header, ':'); p != -1 && bytes.Equal(header[0:p], name) {
				return true
			}
			from = edge
		}
	}
	return false
}
func (r *webOut_) delHeader1(name []byte) (deleted bool) {
	from := uint16(0)
	for i := uint8(1); i < r.nHeaders; {
		edge := r.edges[i]
		if p := bytes.IndexByte(r.fields[from:edge], ':'); bytes.Equal(r.fields[from:from+uint16(p)], name) {
			size := edge - from
			copy(r.fields[from:], r.fields[edge:])
			for j := i + 1; j < r.nHeaders; j++ {
				r.edges[j] -= size
			}
			r.fieldsEdge -= size
			r.nHeaders--
			deleted = true
		} else {
			from = edge
			i++
		}
	}
	return
}
func (r *webOut_) delHeaderAt1(i uint8) {
	if i == 0 {
		BugExitln("delHeaderAt1: i == 0 which must not happen")
	}
	from := r.edges[i-1]
	edge := r.edges[i]
	size := edge - from
	copy(r.fields[from:], r.fields[edge:])
	for j := i + 1; j < r.nHeaders; j++ {
		r.edges[j] -= size
	}
	r.fieldsEdge -= size
	r.nHeaders--
}
func (r *webOut_) _addCRLFHeader1(from int) {
	r.fields[from] = '\r'
	r.fields[from+1] = '\n'
	r.edges[r.nHeaders] = uint16(from + 2)
	r.nHeaders++
}
func (r *webOut_) _addFixedHeader1(name []byte, value []byte) { // used by finalizeHeaders
	r.fieldsEdge += uint16(copy(r.fields[r.fieldsEdge:], name))
	r.fields[r.fieldsEdge] = ':'
	r.fields[r.fieldsEdge+1] = ' '
	r.fieldsEdge += 2
	r.fieldsEdge += uint16(copy(r.fields[r.fieldsEdge:], value))
	r.fields[r.fieldsEdge] = '\r'
	r.fields[r.fieldsEdge+1] = '\n'
	r.fieldsEdge += 2
}

func (r *webOut_) sendChain1() error { // TODO: if conn is TLS, don't use writev as it uses many Write() which might be slower than make+copy+write.
	return r._sendEntireChain1()
	// TODO
	nContentRanges := len(r.contentRanges)
	if nContentRanges == 0 {
		return r._sendEntireChain1()
	}
	// Partial content.
	if !r.asRequest { // as response
		r.outMessage.(Response).SetStatus(StatusPartialContent)
	}
	if nContentRanges == 1 {
		return r._sendSingleRange1()
	} else {
		return r._sendMultiRanges1()
	}
}
func (r *webOut_) _sendEntireChain1() error {
	r.outMessage.finalizeHeaders()
	vector := r._prepareVector1() // waiting to write
	if DebugLevel() >= 2 {
		if r.asRequest {
			Printf("[backend1Stream=%d]=======> ", r.stream.Conn().ID())
		} else {
			Printf("[server1Stream=%d]-------> ", r.stream.Conn().ID())
		}
		Printf("[%s%s%s]\n", vector[0], vector[1], vector[2])
	}
	vFrom, vEdge := 0, 3
	for piece := r.chain.head; piece != nil; piece = piece.next {
		if piece.size == 0 {
			continue
		}
		if piece.IsText() { // plain text
			vector[vEdge] = piece.Text()
			vEdge++
		} else if piece.size <= _16K { // small file, <= 16K
			buffer := GetNK(piece.size) // 4K/16K
			if err := piece.copyTo(buffer); err != nil {
				r.stream.markBroken()
				PutNK(buffer)
				return err
			}
			vector[vEdge] = buffer[0:piece.size]
			vEdge++
			r.vector = vector[vFrom:vEdge]
			if err := r.writeVector1(); err != nil {
				PutNK(buffer)
				return err
			}
			PutNK(buffer)
			vFrom, vEdge = 0, 0
		} else { // large file, > 16K
			if vFrom < vEdge {
				r.vector = vector[vFrom:vEdge]
				if err := r.writeVector1(); err != nil { // texts
					return err
				}
				vFrom, vEdge = 0, 0
			}
			if err := r.writePiece1(piece, false); err != nil { // the file
				return err
			}
		}
	}
	if vFrom < vEdge {
		r.vector = vector[vFrom:vEdge]
		return r.writeVector1()
	}
	return nil
}
func (r *webOut_) _sendSingleRange1() error {
	r.AddContentType(r.rangeType)
	valueBuffer := r.stream.buffer256()
	n := copy(valueBuffer, "bytes ")
	contentRange := r.contentRanges[0]
	n += i64ToDec(contentRange.From, valueBuffer[n:])
	valueBuffer[n] = '-'
	n++
	n += i64ToDec(contentRange.Last-1, valueBuffer[n:])
	valueBuffer[n] = '/'
	n++
	n += i64ToDec(r.contentSize, valueBuffer[n:])
	r.AddHeaderBytes(bytesContentRange, valueBuffer[:n])
	//return r._sendEntireChain1()
	return nil
}
func (r *webOut_) _sendMultiRanges1() error {
	valueBuffer := r.stream.buffer256()
	n := copy(valueBuffer, "multipart/byteranges; boundary=")
	n += copy(valueBuffer[n:], "xsd3lxT9b5c")
	r.AddHeaderBytes(bytesContentType, valueBuffer[:n])
	// TODO
	return nil
}
func (r *webOut_) _prepareVector1() [][]byte {
	var vector [][]byte // waiting for write
	if r.forbidContent {
		vector = r.fixedVector[0:3]
		r.chain.free()
	} else if nPieces := r.chain.Qnty(); nPieces == 1 { // content chain has exactly one piece
		vector = r.fixedVector[0:4]
	} else { // nPieces >= 2
		vector = make([][]byte, 3+nPieces) // TODO(diogin): get from pool? defer pool.put()
	}
	vector[0] = r.outMessage.control()
	vector[1] = r.outMessage.addedHeaders()
	vector[2] = r.outMessage.fixedHeaders()
	return vector
}

func (r *webOut_) echoChain1(inChunked bool) error { // TODO: coalesce text pieces?
	for piece := r.chain.head; piece != nil; piece = piece.next {
		if err := r.writePiece1(piece, inChunked); err != nil {
			return err
		}
	}
	return nil
}

func (r *webOut_) addTrailer1(name []byte, value []byte) bool {
	if len(name) == 0 {
		return false
	}
	trailerSize := len(name) + len(bytesColonSpace) + len(value) + len(bytesCRLF) // name: value\r\n
	if from, _, ok := r.growTrailer(trailerSize); ok {
		from += copy(r.fields[from:], name)
		r.fields[from] = ':'
		r.fields[from+1] = ' '
		from += 2
		from += copy(r.fields[from:], value)
		r.fields[from] = '\r'
		r.fields[from+1] = '\n'
		r.edges[r.nTrailers] = uint16(from + 2)
		r.nTrailers++
		return true
	} else {
		return false
	}
}
func (r *webOut_) trailer1(name []byte) (value []byte, ok bool) {
	if r.nTrailers > 1 && len(name) > 0 {
		from := uint16(0)
		for i := uint8(1); i < r.nTrailers; i++ {
			edge := r.edges[i]
			trailer := r.fields[from:edge]
			if p := bytes.IndexByte(trailer, ':'); p != -1 && bytes.Equal(trailer[0:p], name) {
				return trailer[p+len(bytesColonSpace) : len(trailer)-len(bytesCRLF)], true
			}
			from = edge
		}
	}
	return
}
func (r *webOut_) trailers1() []byte { return r.fields[0:r.fieldsEdge] } // Headers and trailers are not manipulated at the same time, so after headers is sent, r.fields is used by trailers.

func (r *webOut_) proxyPassBytes1(p []byte) error { return r.writeBytes1(p) }

func (r *webOut_) finalizeVague1() error {
	if r.nTrailers == 1 { // no trailers
		return r.writeBytes1(http1BytesZeroCRLFCRLF) // 0\r\n\r\n
	} else { // with trailers
		r.vector = r.fixedVector[0:3]
		r.vector[0] = http1BytesZeroCRLF // 0\r\n
		r.vector[1] = r.trailers1()      // field-name: field-value\r\n
		r.vector[2] = bytesCRLF          // \r\n
		return r.writeVector1()
	}
}

func (r *webOut_) writeHeaders1() error { // used by echo and pass
	r.outMessage.finalizeHeaders()
	r.vector = r.fixedVector[0:3]
	r.vector[0] = r.outMessage.control()
	r.vector[1] = r.outMessage.addedHeaders()
	r.vector[2] = r.outMessage.fixedHeaders()
	if DebugLevel() >= 2 {
		if r.asRequest {
			Printf("[backend1Stream=%d]", r.stream.Conn().ID())
		} else {
			Printf("[server1Stream=%d]", r.stream.Conn().ID())
		}
		Printf("-------> [%s%s%s]\n", r.vector[0], r.vector[1], r.vector[2])
	}
	if err := r.writeVector1(); err != nil {
		return err
	}
	r.fieldsEdge = 0 // now that headers are all sent, r.fields will be used by trailers (if any), so reset it.
	return nil
}
func (r *webOut_) writePiece1(piece *Piece, inChunked bool) error {
	if r.stream.isBroken() {
		return webOutWriteBroken
	}
	if piece.IsText() { // text piece
		return r._writeTextPiece1(piece, inChunked)
	} else {
		return r._writeFilePiece1(piece, inChunked)
	}
}
func (r *webOut_) _writeTextPiece1(piece *Piece, inChunked bool) error {
	if inChunked { // HTTP/1.1 chunked data
		sizeBuffer := r.stream.buffer256() // buffer is enough for chunk size
		n := i64ToHex(piece.size, sizeBuffer)
		sizeBuffer[n] = '\r'
		sizeBuffer[n+1] = '\n'
		n += 2
		r.vector = r.fixedVector[0:3] // we reuse r.vector and r.fixedVector
		r.vector[0] = sizeBuffer[:n]
		r.vector[1] = piece.Text()
		r.vector[2] = bytesCRLF
		return r.writeVector1()
	} else { // HTTP/1.0, or raw data
		return r.writeBytes1(piece.Text())
	}
}
func (r *webOut_) _writeFilePiece1(piece *Piece, inChunked bool) error {
	// file piece. currently we don't use sendfile(2).
	buffer := Get16K() // 16K is a tradeoff between performance and memory consumption.
	defer PutNK(buffer)
	sizeRead := int64(0)
	for {
		if sizeRead == piece.size {
			return nil
		}
		readSize := int64(cap(buffer))
		if sizeLeft := piece.size - sizeRead; sizeLeft < readSize {
			readSize = sizeLeft
		}
		n, err := piece.file.ReadAt(buffer[:readSize], sizeRead)
		sizeRead += int64(n)
		if err != nil && sizeRead != piece.size {
			r.stream.markBroken()
			return err
		}
		if err = r._beforeWrite(); err != nil {
			r.stream.markBroken()
			return err
		}
		if inChunked { // use HTTP/1.1 chunked mode
			sizeBuffer := r.stream.buffer256()
			k := i64ToHex(int64(n), sizeBuffer)
			sizeBuffer[k] = '\r'
			sizeBuffer[k+1] = '\n'
			k += 2
			r.vector = r.fixedVector[0:3]
			r.vector[0] = sizeBuffer[:k]
			r.vector[1] = buffer[:n]
			r.vector[2] = bytesCRLF
			_, err = r.stream.writev(&r.vector)
		} else { // HTTP/1.0, or identity content
			_, err = r.stream.write(buffer[0:n])
		}
		if err = r._slowCheck(err); err != nil {
			return err
		}
	}
}
func (r *webOut_) writeVector1() error {
	if r.stream.isBroken() {
		return webOutWriteBroken
	}
	if len(r.vector) == 1 && len(r.vector[0]) == 0 { // empty data
		return nil
	}
	if err := r._beforeWrite(); err != nil {
		r.stream.markBroken()
		return err
	}
	_, err := r.stream.writev(&r.vector)
	return r._slowCheck(err)
}
func (r *webOut_) writeBytes1(p []byte) error {
	if r.stream.isBroken() {
		return webOutWriteBroken
	}
	if len(p) == 0 { // empty data
		return nil
	}
	if err := r._beforeWrite(); err != nil {
		r.stream.markBroken()
		return err
	}
	_, err := r.stream.write(p)
	return r._slowCheck(err)
}

// HTTP/1.x webSocket

func (s *webSocket_) todo1() {
	// TODO
}

//////////////////////////////////////// HTTP/1.x protocol elements ////////////////////////////////////////

var http1Template = [16]byte{'H', 'T', 'T', 'P', '/', '1', '.', '1', ' ', 'N', 'N', 'N', ' ', 'X', '\r', '\n'}
var http1Controls = [...][]byte{ // size: 512*24B=12K. keep sync with http2Control and http3Control!
	// 1XX
	StatusContinue:           []byte("HTTP/1.1 100 Continue\r\n"),
	StatusSwitchingProtocols: []byte("HTTP/1.1 101 Switching Protocols\r\n"),
	StatusProcessing:         []byte("HTTP/1.1 102 Processing\r\n"),
	StatusEarlyHints:         []byte("HTTP/1.1 103 Early Hints\r\n"),
	// 2XX
	StatusOK:                         []byte("HTTP/1.1 200 OK\r\n"),
	StatusCreated:                    []byte("HTTP/1.1 201 Created\r\n"),
	StatusAccepted:                   []byte("HTTP/1.1 202 Accepted\r\n"),
	StatusNonAuthoritativeInfomation: []byte("HTTP/1.1 203 Non-Authoritative Information\r\n"),
	StatusNoContent:                  []byte("HTTP/1.1 204 No Content\r\n"),
	StatusResetContent:               []byte("HTTP/1.1 205 Reset Content\r\n"),
	StatusPartialContent:             []byte("HTTP/1.1 206 Partial Content\r\n"),
	StatusMultiStatus:                []byte("HTTP/1.1 207 Multi-Status\r\n"),
	StatusAlreadyReported:            []byte("HTTP/1.1 208 Already Reported\r\n"),
	StatusIMUsed:                     []byte("HTTP/1.1 226 IM Used\r\n"),
	// 3XX
	StatusMultipleChoices:   []byte("HTTP/1.1 300 Multiple Choices\r\n"),
	StatusMovedPermanently:  []byte("HTTP/1.1 301 Moved Permanently\r\n"),
	StatusFound:             []byte("HTTP/1.1 302 Found\r\n"),
	StatusSeeOther:          []byte("HTTP/1.1 303 See Other\r\n"),
	StatusNotModified:       []byte("HTTP/1.1 304 Not Modified\r\n"),
	StatusUseProxy:          []byte("HTTP/1.1 305 Use Proxy\r\n"),
	StatusTemporaryRedirect: []byte("HTTP/1.1 307 Temporary Redirect\r\n"),
	StatusPermanentRedirect: []byte("HTTP/1.1 308 Permanent Redirect\r\n"),
	// 4XX
	StatusBadRequest:                  []byte("HTTP/1.1 400 Bad Request\r\n"),
	StatusUnauthorized:                []byte("HTTP/1.1 401 Unauthorized\r\n"),
	StatusPaymentRequired:             []byte("HTTP/1.1 402 Payment Required\r\n"),
	StatusForbidden:                   []byte("HTTP/1.1 403 Forbidden\r\n"),
	StatusNotFound:                    []byte("HTTP/1.1 404 Not Found\r\n"),
	StatusMethodNotAllowed:            []byte("HTTP/1.1 405 Method Not Allowed\r\n"),
	StatusNotAcceptable:               []byte("HTTP/1.1 406 Not Acceptable\r\n"),
	StatusProxyAuthenticationRequired: []byte("HTTP/1.1 407 Proxy Authentication Required\r\n"),
	StatusRequestTimeout:              []byte("HTTP/1.1 408 Request Timeout\r\n"),
	StatusConflict:                    []byte("HTTP/1.1 409 Conflict\r\n"),
	StatusGone:                        []byte("HTTP/1.1 410 Gone\r\n"),
	StatusLengthRequired:              []byte("HTTP/1.1 411 Length Required\r\n"),
	StatusPreconditionFailed:          []byte("HTTP/1.1 412 Precondition Failed\r\n"),
	StatusContentTooLarge:             []byte("HTTP/1.1 413 Content Too Large\r\n"),
	StatusURITooLong:                  []byte("HTTP/1.1 414 URI Too Long\r\n"),
	StatusUnsupportedMediaType:        []byte("HTTP/1.1 415 Unsupported Media Type\r\n"),
	StatusRangeNotSatisfiable:         []byte("HTTP/1.1 416 Range Not Satisfiable\r\n"),
	StatusExpectationFailed:           []byte("HTTP/1.1 417 Expectation Failed\r\n"),
	StatusMisdirectedRequest:          []byte("HTTP/1.1 421 Misdirected Request\r\n"),
	StatusUnprocessableEntity:         []byte("HTTP/1.1 422 Unprocessable Entity\r\n"),
	StatusLocked:                      []byte("HTTP/1.1 423 Locked\r\n"),
	StatusFailedDependency:            []byte("HTTP/1.1 424 Failed Dependency\r\n"),
	StatusTooEarly:                    []byte("HTTP/1.1 425 Too Early\r\n"),
	StatusUpgradeRequired:             []byte("HTTP/1.1 426 Upgrade Required\r\n"),
	StatusPreconditionRequired:        []byte("HTTP/1.1 428 Precondition Required\r\n"),
	StatusTooManyRequests:             []byte("HTTP/1.1 429 Too Many Requests\r\n"),
	StatusRequestHeaderFieldsTooLarge: []byte("HTTP/1.1 431 Request Header Fields Too Large\r\n"),
	StatusUnavailableForLegalReasons:  []byte("HTTP/1.1 451 Unavailable For Legal Reasons\r\n"),
	// 5XX
	StatusInternalServerError:           []byte("HTTP/1.1 500 Internal Server Error\r\n"),
	StatusNotImplemented:                []byte("HTTP/1.1 501 Not Implemented\r\n"),
	StatusBadGateway:                    []byte("HTTP/1.1 502 Bad Gateway\r\n"),
	StatusServiceUnavailable:            []byte("HTTP/1.1 503 Service Unavailable\r\n"),
	StatusGatewayTimeout:                []byte("HTTP/1.1 504 Gateway Timeout\r\n"),
	StatusHTTPVersionNotSupported:       []byte("HTTP/1.1 505 HTTP Version Not Supported\r\n"),
	StatusVariantAlsoNegotiates:         []byte("HTTP/1.1 506 Variant Also Negotiates\r\n"),
	StatusInsufficientStorage:           []byte("HTTP/1.1 507 Insufficient Storage\r\n"),
	StatusLoopDetected:                  []byte("HTTP/1.1 508 Loop Detected\r\n"),
	StatusNotExtended:                   []byte("HTTP/1.1 510 Not Extended\r\n"),
	StatusNetworkAuthenticationRequired: []byte("HTTP/1.1 511 Network Authentication Required\r\n"),
}

var ( // HTTP/1.x byteses
	http1BytesContinue             = []byte("HTTP/1.1 100 Continue\r\n\r\n")
	http1BytesConnectionClose      = []byte("connection: close\r\n")
	http1BytesConnectionKeepAlive  = []byte("connection: keep-alive\r\n")
	http1BytesContentTypeStream    = []byte("content-type: application/octet-stream\r\n")
	http1BytesContentTypeHTMLUTF8  = []byte("content-type: text/html; charset=utf-8\r\n")
	http1BytesTransferChunked      = []byte("transfer-encoding: chunked\r\n")
	http1BytesVaryEncoding         = []byte("vary: accept-encoding\r\n")
	http1BytesLocationHTTP         = []byte("location: http://")
	http1BytesLocationHTTPS        = []byte("location: https://")
	http1BytesFixedRequestHeaders  = []byte("client: gorox\r\n\r\n")
	http1BytesFixedResponseHeaders = []byte("server: gorox\r\n\r\n")
	http1BytesZeroCRLF             = []byte("0\r\n")
	http1BytesZeroCRLFCRLF         = []byte("0\r\n\r\n")
)
