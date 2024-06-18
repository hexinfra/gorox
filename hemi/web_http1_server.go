// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// HTTP/1 server implementation. See RFC 9112.

// Both HTTP/1.0 and HTTP/1.1 are supported. Pipelining is supported but not optimized because it's rarely used.

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

// poolServer1Conn is the server-side HTTP/1 connection pool.
var poolServer1Conn sync.Pool

func getServer1Conn(id int64, gate *httpxGate, netConn net.Conn, rawConn syscall.RawConn) *server1Conn {
	var serverConn *server1Conn
	if x := poolServer1Conn.Get(); x == nil {
		serverConn = new(server1Conn)
		stream := &serverConn.stream
		stream.conn = serverConn
		req, resp := &stream.request, &stream.response
		req.shell = req
		req.stream = stream
		resp.shell = resp
		resp.stream = stream
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

// server1Conn is the server-side HTTP/1 connection.
type server1Conn struct {
	// Parent
	ServerConn_
	// Mixins
	_httpConn_
	// Assocs
	stream server1Stream // an http/1 connection has exactly one stream
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	server    *httpxServer
	gate      *httpxGate
	netConn   net.Conn        // the connection (UDS/TCP/TLS)
	rawConn   syscall.RawConn // for syscall, only when netConn is UDS/TCP
	closeSafe bool            // if false, send a FIN first to avoid TCP's RST following immediate close(). true by default
	// Conn states (zeros)
}

func (c *server1Conn) onGet(id int64, gate *httpxGate, netConn net.Conn, rawConn syscall.RawConn) {
	c.ServerConn_.OnGet(id)
	c._httpConn_.onGet()

	c.server = gate.server
	c.gate = gate
	c.netConn = netConn
	c.rawConn = rawConn
	c.closeSafe = true

	req := &c.stream.request
	req.input = req.stockInput[:] // input is conn scoped but put in stream scoped request for convenience
}
func (c *server1Conn) onPut() {
	req := &c.stream.request
	if cap(req.input) != cap(req.stockInput) { // fetched from pool
		// req.input is conn scoped but put in stream scoped c.request for convenience
		PutNK(req.input)
		req.input = nil
	}
	req.inputNext, req.inputEdge = 0, 0 // inputNext and inputEdge are conn scoped but put in stream scoped request for convenience

	c.netConn = nil
	c.rawConn = nil
	c.gate = nil
	c.server = nil

	c._httpConn_.onPut()
	c.ServerConn_.OnPut()
}

func (c *server1Conn) IsTLS() bool { return c.server.IsTLS() }
func (c *server1Conn) IsUDS() bool { return c.server.IsUDS() }

func (c *server1Conn) MakeTempName(p []byte, unixTime int64) int {
	return makeTempName(p, int64(c.server.Stage().ID()), c.id, unixTime, c.counter.Add(1))
}

func (c *server1Conn) serve() { // runner
	defer putServer1Conn(c)

	stream := &c.stream
	for c.persistent { // each queued stream
		stream.onUse()
		stream.execute()
		stream.onEnd()
	}

	// RFC 7230 (section 6.6):
	//
	// If a server performs an immediate close of a TCP connection, there is
	// a significant risk that the client will not be able to read the last
	// HTTP response.  If the server receives additional data from the
	// client on a fully closed connection, such as another request that was
	// sent by the client before receiving the server's response, the
	// server's TCP stack will send a reset packet to the client;
	// unfortunately, the reset packet might erase the client's
	// unacknowledged input buffers before they can be read and interpreted
	// by the client's HTTP parser.
	//
	// To avoid the TCP reset problem, servers typically close a connection
	// in stages.  First, the server performs a half-close by closing only
	// the write side of the read/write connection.  The server then
	// continues to read from the connection until it receives a
	// corresponding close by the client, or until the server is reasonably
	// certain that its own TCP stack has received the client's
	// acknowledgement of the packet(s) containing the server's last
	// response.  Finally, the server fully closes the connection.
	netConn := c.netConn
	if !c.closeSafe {
		if c.IsTLS() {
			netConn.(*tls.Conn).CloseWrite()
		} else if c.IsUDS() {
			netConn.(*net.UnixConn).CloseWrite()
		} else {
			netConn.(*net.TCPConn).CloseWrite()
		}
		time.Sleep(time.Second)
	}
	netConn.Close()
	c.gate.OnConnClosed()
}

// server1Stream is the server-side HTTP/1 stream.
type server1Stream struct {
	// Mixins
	_httpStream_
	// Assocs
	conn     *server1Conn
	request  server1Request  // the server-side http/1 request.
	response server1Response // the server-side http/1 response.
	socket   *server1Socket  // the server-side http/1 websocket.
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (s *server1Stream) onUse() { // for non-zeros
	s._httpStream_.onUse()

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

	s._httpStream_.onEnd()
}

func (s *server1Stream) execute() {
	req, resp := &s.request, &s.response

	req.recvHead()

	if req.HeadResult() != StatusOK { // receiving request error
		s.serveAbnormal(req, resp)
		return
	}

	if req.methodCode == MethodCONNECT {
		req.headResult, req.failReason = StatusNotImplemented, "tcp over http is not implemented here"
		s.serveAbnormal(req, resp)
		return
	}

	conn := s.conn
	server := conn.server
	// RFC 9112:
	// If the server's configuration provides for a fixed URI scheme, or a
	// scheme is provided by a trusted outbound gateway, that scheme is
	// used for the target URI. This is common in large-scale deployments
	// because a gateway server will receive the client's connection context
	// and replace that with their own connection to the inbound server.
	// Otherwise, if the request is received over a secured connection, the
	// target URI's scheme is "https"; if not, the scheme is "http".
	if server.forceScheme != -1 { // forceScheme is set explicitly
		req.schemeCode = uint8(server.forceScheme)
	} else { // scheme is not forced
		if conn.IsTLS() {
			if req.schemeCode == SchemeHTTP && server.adjustScheme {
				req.schemeCode = SchemeHTTPS
			}
		} else { // not secured
			if req.schemeCode == SchemeHTTPS && server.adjustScheme {
				req.schemeCode = SchemeHTTP
			}
		}
	}

	webapp := server.findApp(req.UnsafeHostname())

	if webapp == nil {
		req.headResult, req.failReason = StatusNotFound, "target webapp is not found in this server"
		s.serveAbnormal(req, resp)
		return
	}
	if !webapp.isDefault && !bytes.Equal(req.UnsafeColonPort(), server.ColonPortBytes()) {
		req.headResult, req.failReason = StatusNotFound, "authoritative webapp is not found in this server"
		s.serveAbnormal(req, resp)
		return
	}

	req.webapp = webapp
	resp.webapp = webapp

	if !req.upgradeSocket { // exchan mode
		if req.formKind != httpFormNotForm {
			if req.formKind == httpFormMultipart { // we allow a larger content size for uploading through multipart/form-data (large files are written to disk).
				req.maxContentSize = webapp.maxUpfileSize
			} else { // application/x-www-form-urlencoded is limited in a smaller size.
				req.maxContentSize = int64(server.MaxMemoryContentSize())
			}
		}
		if req.contentSize > req.maxContentSize {
			if req.expectContinue {
				req.headResult = StatusExpectationFailed
			} else {
				req.headResult, req.failReason = StatusContentTooLarge, "content size exceeds webapp's limit"
			}
			s.serveAbnormal(req, resp)
			return
		}

		// Prepare the response according to the request
		if req.methodCode == MethodHEAD {
			resp.forbidContent = true
		}

		if req.expectContinue && !s.writeContinue() {
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
		if req.expectContinue && !s.writeContinue() {
			return
		}

		s.executeSocket()

		conn.persistent = false // explicitly
	}
}

func (s *server1Stream) writeContinue() bool { // 100 continue
	conn := s.conn
	// This is an interim response, write directly.
	if s.setWriteDeadline(time.Now().Add(conn.server.WriteTimeout())) == nil {
		if _, err := s.write(http1BytesContinue); err == nil {
			return true
		}
	}
	// i/o error
	conn.persistent = false
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
func (s *server1Stream) serveAbnormal(req *server1Request, resp *server1Response) { // 4xx & 5xx
	conn := s.conn
	if DebugLevel() >= 2 {
		Printf("server=%s gate=%d conn=%d headResult=%d\n", conn.server.Name(), conn.gate.ID(), conn.id, s.request.headResult)
	}
	conn.persistent = false // close anyway.

	status := req.headResult
	if status == -1 || (status == StatusRequestTimeout && !req.gotInput) {
		return // send nothing.
	}
	// So we need to send something...
	if status == StatusContentTooLarge || status == StatusURITooLong || status == StatusRequestHeaderFieldsTooLarge {
		// The receiving side may has data when we close the connection
		conn.closeSafe = false
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
	if s.setWriteDeadline(time.Now().Add(conn.server.WriteTimeout())) == nil {
		s.writev(&resp.vector)
	}
}

func (s *server1Stream) executeSocket() { // upgrade: websocket
	// TODO(diogin): implementation (RFC 6455).
	// NOTICE: use idle timeout or clear read timeout otherwise?
	s.write([]byte("HTTP/1.1 501 Not Implemented\r\nConnection: close\r\n\r\n"))
}

func (s *server1Stream) httpServend() httpServend { return s.conn.server }
func (s *server1Stream) httpConn() httpConn       { return s.conn }
func (s *server1Stream) remoteAddr() net.Addr     { return s.conn.netConn.RemoteAddr() }

func (s *server1Stream) markBroken()    { s.conn.markBroken() }
func (s *server1Stream) isBroken() bool { return s.conn.isBroken() }

func (s *server1Stream) setReadDeadline(deadline time.Time) error {
	conn := s.conn
	if deadline.Sub(conn.lastRead) >= time.Second {
		if err := conn.netConn.SetReadDeadline(deadline); err != nil {
			return err
		}
		conn.lastRead = deadline
	}
	return nil
}
func (s *server1Stream) setWriteDeadline(deadline time.Time) error {
	conn := s.conn
	if deadline.Sub(conn.lastWrite) >= time.Second {
		if err := conn.netConn.SetWriteDeadline(deadline); err != nil {
			return err
		}
		conn.lastWrite = deadline
	}
	return nil
}

func (s *server1Stream) read(p []byte) (int, error)     { return s.conn.netConn.Read(p) }
func (s *server1Stream) readFull(p []byte) (int, error) { return io.ReadFull(s.conn.netConn, p) }
func (s *server1Stream) write(p []byte) (int, error)    { return s.conn.netConn.Write(p) }
func (s *server1Stream) writev(vector *net.Buffers) (int64, error) {
	return vector.WriteTo(s.conn.netConn)
}

// server1Request is the server-side HTTP/1 request.
type server1Request struct { // incoming. needs parsing
	// Parent
	serverRequest_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *server1Request) recvHead() { // control + headers
	// The entire request head must be received in one timeout
	if err := r._beforeRead(&r.recvTime); err != nil {
		r.headResult = -1
		return
	}
	if r.inputEdge == 0 && !r.growHead1() { // r.inputEdge == 0 means r.input is empty, so we must fill it
		// r.headResult is set.
		return
	}
	if !r._recvControl() || !r.recvHeaders1() || !r.examineHead() {
		// r.headResult is set.
		return
	}
	r.cleanInput()
	if DebugLevel() >= 2 {
		Printf("[server1Stream=%d]<------- [%s]\n", r.stream.httpConn().ID(), r.input[r.head.from:r.head.edge])
	}
}
func (r *server1Request) _recvControl() bool { // method SP request-target SP HTTP-version CRLF
	r.pBack, r.pFore = 0, 0

	// method = token
	// token = 1*tchar
	hash := uint16(0)
	for {
		if b := r.input[r.pFore]; httpTchar[b] != 0 {
			hash += uint16(b)
			if r.pFore++; r.pFore == r.inputEdge && !r.growHead1() {
				return false
			}
		} else if b == ' ' {
			break
		} else {
			r.headResult, r.failReason = StatusBadRequest, "invalid character in method"
			return false
		}
	}
	if r.pBack == r.pFore {
		r.headResult, r.failReason = StatusBadRequest, "empty method"
		return false
	}
	r.gotInput = true
	r.method.set(r.pBack, r.pFore)
	r.recognizeMethod(r.input[r.pBack:r.pFore], hash)
	// Skip SP after method
	if r.pFore++; r.pFore == r.inputEdge && !r.growHead1() {
		return false
	}

	// Now r.pFore is at request-target.
	r.pBack = r.pFore
	// request-target = absolute-form / origin-form / authority-form / asterisk-form
	if b := r.input[r.pFore]; b != '*' && r.methodCode != MethodCONNECT { // absolute-form / origin-form
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
				if b := r.input[r.pFore]; b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '+' || b == '-' || b == '.' {
					// Do nothing
				} else if b >= 'A' && b <= 'Z' {
					// RFC 7230 (section 2.7.3.  http and https URI Normalization and Comparison):
					// The scheme and host are case-insensitive and normally provided in lowercase;
					// all other components are compared in a case-sensitive manner.
					r.input[r.pFore] = b + 0x20 // to lower
				} else if b == ':' {
					break
				} else {
					r.headResult, r.failReason = StatusBadRequest, "bad scheme"
					return false
				}
				if r.pFore++; r.pFore == r.inputEdge && !r.growHead1() {
					return false
				}
			}
			if scheme := r.input[r.pBack:r.pFore]; bytes.Equal(scheme, bytesHTTP) {
				r.schemeCode = SchemeHTTP
			} else if bytes.Equal(scheme, bytesHTTPS) {
				r.schemeCode = SchemeHTTPS
			} else {
				r.headResult, r.failReason = StatusBadRequest, "unknown scheme"
				return false
			}
			// Skip ':'
			if r.pFore++; r.pFore == r.inputEdge && !r.growHead1() {
				return false
			}
			if r.input[r.pFore] != '/' {
				r.headResult, r.failReason = StatusBadRequest, "bad first slash"
				return false
			}
			// Skip '/'
			if r.pFore++; r.pFore == r.inputEdge && !r.growHead1() {
				return false
			}
			if r.input[r.pFore] != '/' {
				r.headResult, r.failReason = StatusBadRequest, "bad second slash"
				return false
			}
			// Skip '/'
			if r.pFore++; r.pFore == r.inputEdge && !r.growHead1() {
				return false
			}
			// authority = host [ ":" port ]
			// host = IP-literal / IPv4address / reg-name
			r.pBack = r.pFore
			for {
				if b = r.input[r.pFore]; b >= 'A' && b <= 'Z' {
					r.input[r.pFore] = b + 0x20 // to lower
				} else if b == '/' || b == ' ' {
					break
				}
				if r.pFore++; r.pFore == r.inputEdge && !r.growHead1() {
					return false
				}
			}
			if r.pBack == r.pFore {
				r.headResult, r.failReason = StatusBadRequest, "empty authority is not allowed"
				return false
			}
			if !r.parseAuthority(r.pBack, r.pFore, true) { // save = true
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
			r.pBack = r.pFore // at '/'.
		}
		// RFC 7230 (5.3.1.  origin-form)
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
		// URI as the request-target.  If the target URI's path component is
		// empty, the client MUST send "/" as the path within the origin-form of
		// request-target.  A Host header field is also sent, as defined in
		// Section 5.4.
		var (
			state = 1   // in path
			octet byte  // byte value of %xx
			qsOff int32 // offset of query string, if exists
		)
		query := &r.mainPair
		query.zero()
		query.kind = pairQuery
		query.place = placeArray // all received queries are placed in r.array because queries are decoded

		// r.pFore is at '/'.
	uri:
		for { // TODO: use a better algorithm to improve performance, state machine might be slow here.
			b := r.input[r.pFore]
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
					qsOff = r.pFore - r.pBack
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
					query.hash += uint16(b)
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
					query.hash = 0 // reset for next query
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
						query.hash += uint16(octet)
					} else if octet == 0x00 && state == 0x10 { // For security reasons, we reject "\x00" in path.
						r.headResult, r.failReason = StatusBadRequest, "malformed path"
						return false
					}
					r.arrayPush(octet)
					state >>= 4 // restore previous state
				}
			}
			if r.pFore++; r.pFore == r.inputEdge && !r.growHead1() {
				return false
			}
		}
		if state == 1 { // path ends without a '?'
			r.path = r.array[0:r.arrayEdge]
		} else if state == 2 { // in query string and no '=' found
			r.queryString.set(r.pBack+qsOff, r.pFore)
			// Since there is no '=', we ignore this query
		} else if state == 3 { // in query string and no '&' found
			r.queryString.set(r.pBack+qsOff, r.pFore)
			query.value.edge = r.arrayEdge
			if query.nameSize > 0 && !r.addQuery(query) {
				return false
			}
		} else { // incomplete pct-encoded
			r.headResult, r.failReason = StatusBadRequest, "incomplete pct-encoded"
			return false
		}

		r.uri.set(r.pBack, r.pFore)
		if qsOff == 0 {
			r.encodedPath = r.uri
		} else {
			r.encodedPath.set(r.pBack, r.pBack+qsOff)
		}
		r.cleanPath()
	} else if b == '*' { // OPTIONS *, asterisk-form
		r.targetForm = httpTargetAsterisk
		// RFC 7230 (section 5.3.4):
		// The asterisk-form of request-target is only used for a server-wide
		// OPTIONS request (Section 4.3.7 of [RFC7231]).
		if r.methodCode != MethodOPTIONS {
			r.headResult, r.failReason = StatusBadRequest, "asterisk-form is only used by OPTIONS method"
			return false
		}
		// Skip '*'. We don't use it as uri! Instead, we use '/'. To test OPTIONS *, test r.asteriskOptions set below.
		if r.pFore++; r.pFore == r.inputEdge && !r.growHead1() {
			return false
		}
		r.asteriskOptions = true
		// Expect SP
		if r.input[r.pFore] != ' ' {
			r.headResult, r.failReason = StatusBadRequest, "malformed asterisk-form"
			return false
		}
		// RFC 7230 (section 5.5):
		// If the request-target is in authority-form or asterisk-form, the
		// effective request URI's combined path and query component is empty.
	} else { // r.methodCode == MethodCONNECT, authority-form
		r.targetForm = httpTargetAuthority
		// RFC 7230 (section 5.3.3. authority-form:
		// The authority-form of request-target is only used for CONNECT
		// requests (Section 4.3.6 of [RFC7231]).
		//
		//   authority-form = authority
		//   authority      = host [ ":" port ]
		//
		// When making a CONNECT request to establish a tunnel through one or
		// more proxies, a client MUST send only the target URI's authority
		// component (excluding any userinfo and its "@" delimiter) as the
		// request-target.
		for {
			if b := r.input[r.pFore]; b >= 'A' && b <= 'Z' {
				r.input[r.pFore] = b + 0x20 // to lower
			} else if b == ' ' {
				break
			}
			if r.pFore++; r.pFore == r.inputEdge && !r.growHead1() {
				return false
			}
		}
		if r.pBack == r.pFore {
			r.headResult, r.failReason = StatusBadRequest, "empty authority is not allowed"
			return false
		}
		if !r.parseAuthority(r.pBack, r.pFore, true) { // save = true
			r.headResult, r.failReason = StatusBadRequest, "invalid authority"
			return false
		}
		// RFC 7230 (section 5.5):
		// If the request-target is in authority-form or asterisk-form, the
		// effective request URI's combined path and query component is empty.
	}

beforeVersion: // r.pFore is at ' '.
	// Skip SP before HTTP-version
	if r.pFore++; r.pFore == r.inputEdge && !r.growHead1() {
		return false
	}

	// Now r.pFore is at HTTP-version.
	r.pBack = r.pFore
	// HTTP-version = HTTP-name "/" DIGIT "." DIGIT
	// HTTP-name = %x48.54.54.50 ; "HTTP", case-sensitive
	if have := r.inputEdge - r.pFore; have >= 9 {
		// r.pFore -> EOL
		// r.inputEdge -> after EOL or more
		r.pFore += 8
	} else { // have < 9, but len("HTTP/1.X\n") = 9.
		// r.pFore at 'H' -> EOL
		// r.inputEdge at "TTP/1.X\n" -> after EOL
		r.pFore = r.inputEdge - 1
		for i, n := int32(0), 9-have; i < n; i++ {
			if r.pFore++; r.pFore == r.inputEdge && !r.growHead1() {
				return false
			}
		}
	}
	if version := r.input[r.pBack:r.pFore]; bytes.Equal(version, bytesHTTP1_1) {
		r.httpVersion = Version1_1
	} else if bytes.Equal(version, bytesHTTP1_0) {
		r.httpVersion = Version1_0
	} else { // i don't believe there will be a HTTP/1.2 in the future.
		r.headResult = StatusHTTPVersionNotSupported
		return false
	}
	if r.input[r.pFore] == '\r' {
		if r.pFore++; r.pFore == r.inputEdge && !r.growHead1() {
			return false
		}
	}
	if r.input[r.pFore] != '\n' {
		r.headResult, r.failReason = StatusBadRequest, "bad eol of start line"
		return false
	}
	r.receiving = httpSectionHeaders
	// Skip '\n'
	if r.pFore++; r.pFore == r.inputEdge && !r.growHead1() {
		return false
	}

	return true
}
func (r *server1Request) cleanInput() {
	// r.pFore is at the beginning of content (if exists) or next request (if exists and is pipelined).
	if r.contentSize == -1 { // no content
		r.contentReceived = true   // we treat it as "received"
		r.formReceived = true      // set anyway
		if r.pFore < r.inputEdge { // still has data, stream is pipelined
			r.inputNext = r.pFore // mark the beginning of the next request
		} else { // r.pFore == r.inputEdge, no data anymore
			r.inputNext, r.inputEdge = 0, 0 // reset
		}
		return
	}
	// content exists (sized or vague)
	r.imme.set(r.pFore, r.inputEdge)
	if r.contentSize >= 0 { // sized mode
		immeSize := int64(r.imme.size())
		if immeSize == 0 || immeSize <= r.contentSize {
			r.inputNext, r.inputEdge = 0, 0 // reset
		}
		if immeSize >= r.contentSize {
			r.contentReceived = true
			edge := r.pFore + int32(r.contentSize)
			if immeSize > r.contentSize { // still has data, streams are pipelined
				r.imme.set(r.pFore, edge)
				r.inputNext = edge // mark the beginning of next request
			}
			r.receivedSize = r.contentSize        // content is received entirely.
			r.contentText = r.input[r.pFore:edge] // exact.
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

// server1Response is the server-side HTTP/1 response.
type server1Response struct { // outgoing. needs building
	// Parent
	serverResponse_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *server1Response) control() []byte { // HTTP/1.1 xxx ?
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
	// TODO: remove colonPort if colonPort is default?
	colonPort := r.request.UnsafeColonPort()
	headerSize += len(hostname) + len(colonPort) + len(r.request.UnsafeURI()) + len(bytesCRLF)
	if from, _, ok := r.growHeader(headerSize); ok {
		from += copy(r.fields[from:], prefix)
		from += copy(r.fields[from:], hostname) // this is almost always configured, not client provided
		from += copy(r.fields[from:], colonPort)
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
func (r *server1Response) setConnectionClose() { r.stream.httpConn().setPersistent(false) }

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

func (r *server1Response) proxyPass1xx(resp backendResponse) bool {
	resp.delHopHeaders()
	r.status = resp.Status()
	if !resp.forHeaders(func(header *pair, name []byte, value []byte) bool {
		return r.insertHeader(header.hash, name, value)
	}) {
		return false
	}
	r.vector = r.fixedVector[0:3]
	r.vector[0] = r.control()
	r.vector[1] = r.addedHeaders()
	r.vector[2] = bytesCRLF
	// 1xx has no content.
	if r.writeVector1() != nil {
		return false
	}
	// For next use.
	r.onEnd()
	r.onUse(Version1_1)
	return true
}
func (r *server1Response) passHeaders() error       { return r.writeHeaders1() }
func (r *server1Response) passBytes(p []byte) error { return r.passBytes1(p) }

func (r *server1Response) finalizeHeaders() { // add at most 256 bytes
	// date: Sun, 06 Nov 1994 08:49:37 GMT\r\n
	if r.iDate == 0 {
		r.fieldsEdge += uint16(r.stream.httpServend().Stage().Clock().writeDate1(r.fields[r.fieldsEdge:]))
	}
	// expires: Sun, 06 Nov 1994 08:49:37 GMT\r\n
	if r.unixTimes.expires >= 0 {
		r.fieldsEdge += uint16(clockWriteHTTPDate1(r.fields[r.fieldsEdge:], bytesExpires, r.unixTimes.expires))
	}
	// last-modified: Sun, 06 Nov 1994 08:49:37 GMT\r\n
	if r.unixTimes.lastModified >= 0 {
		r.fieldsEdge += uint16(clockWriteHTTPDate1(r.fields[r.fieldsEdge:], bytesLastModified, r.unixTimes.lastModified))
	}
	conn := r.stream.httpConn()
	if r.contentSize != -1 { // with content
		if !r.forbidFraming {
			if !r.isVague() { // content-length: >=0\r\n
				sizeBuffer := r.stream.buffer256() // enough for content-length
				n := i64ToDec(r.contentSize, sizeBuffer)
				r._addFixedHeader1(bytesContentLength, sizeBuffer[:n])
			} else if r.request.VersionCode() == Version1_1 { // transfer-encoding: chunked\r\n
				r.fieldsEdge += uint16(copy(r.fields[r.fieldsEdge:], http1BytesTransferChunked))
			} else {
				// RFC 7230 (section 3.3.1): A server MUST NOT send a
				// response containing Transfer-Encoding unless the corresponding
				// request indicates HTTP/1.1 (or later).
				conn.setPersistent(false) // close conn anyway for HTTP/1.0
			}
		}
		// content-type: text/html; charset=utf-8\r\n
		if r.iContentType == 0 {
			r.fieldsEdge += uint16(copy(r.fields[r.fieldsEdge:], http1BytesContentTypeHTMLUTF8))
		}
	}
	if conn.isPersistent() { // connection: keep-alive\r\n
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

// poolServer1Socket
var poolServer1Socket sync.Pool

func getServer1Socket(stream *server1Stream) *server1Socket {
	return nil
}
func putServer1Socket(socket *server1Socket) {
}

// server1Socket is the server-side HTTP/1 websocket.
type server1Socket struct {
	// Parent
	serverSocket_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (s *server1Socket) onUse() {
	s.serverSocket_.onUse()
}
func (s *server1Socket) onEnd() {
	s.serverSocket_.onEnd()
}
