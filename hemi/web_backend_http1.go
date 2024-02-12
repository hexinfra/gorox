// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HTTP/1 backend implementation. See RFC 9112.

// Only HTTP/1.1 is used. For simplicity, HTTP/1.1 pipelining is not used.

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

func init() {
	RegisterBackend("h1Backend", func(name string, stage *Stage) Backend {
		b := new(H1Backend)
		b.onCreate(name, stage)
		return b
	})
}

// H1Backend
type H1Backend struct {
	// Mixins
	webBackend_[*h1Node]
	// States
}

func (b *H1Backend) onCreate(name string, stage *Stage) {
	b.webBackend_.onCreate(name, stage, b)
}

func (b *H1Backend) OnConfigure() {
	b.webBackend_.onConfigure(b)
}
func (b *H1Backend) OnPrepare() {
	b.webBackend_.onPrepare(b, len(b.nodes))
}

func (b *H1Backend) createNode(id int32) *h1Node {
	node := new(h1Node)
	node.init(id, b)
	return node
}

func (b *H1Backend) FetchConn() (*H1Conn, error) {
	node := b.nodes[b.getNext()]
	return node.fetchConn()
}
func (b *H1Backend) StoreConn(conn *H1Conn) {
	conn.node.(*h1Node).storeConn(conn)
}

// h1Node is a node in H1Backend.
type h1Node struct {
	// Mixins
	Node_
	// Assocs
	backend *H1Backend
	// States
}

func (n *h1Node) init(id int32, backend *H1Backend) {
	n.Node_.init(id)
	n.backend = backend
}

func (n *h1Node) setTLS() {
	n.Node_.setTLS()
	n.tlsConfig.InsecureSkipVerify = true
	n.tlsConfig.NextProtos = []string{"http/1.1"}
}

func (n *h1Node) Maintain() { // runner
	n.Loop(time.Second, func(now time.Time) {
		// TODO: health check, markUp()
	})
	n.markDown()
	if size := n.closeFree(); size > 0 {
		n.IncSub(0 - size)
	}
	n.WaitSubs() // conns
	if Debug() >= 2 {
		Printf("h1Node=%d done\n", n.id)
	}
	n.backend.SubDone()
}

func (n *h1Node) fetchConn() (*H1Conn, error) {
	conn := n.pullConn()
	down := n.isDown()
	if conn != nil {
		h1Conn := conn.(*H1Conn)
		if h1Conn.isAlive() && !h1Conn.reachLimit() && !down {
			return h1Conn, nil
		}
		n.closeConn(h1Conn)
	}
	if down {
		return nil, errNodeDown
	}

	if n.udsMode {
		return n._fetchUDS()
	} else if n.tlsMode {
		return n._fetchTLS()
	} else {
		return n._fetchTCP()
	}
}
func (n *h1Node) _fetchTCP() (*H1Conn, error) {
	// TODO: dynamic address names?
	netConn, err := net.DialTimeout("tcp", n.address, n.backend.dialTimeout)
	if err != nil {
		n.markDown()
		return nil, err
	}
	if Debug() >= 2 {
		Printf("h1Node=%d dial %s OK!\n", n.id, n.address)
	}
	connID := n.backend.nextConnID()
	rawConn, err := netConn.(*net.TCPConn).SyscallConn()
	if err != nil {
		netConn.Close()
		return nil, err
	}
	n.IncSub(1)
	return getH1Conn(connID, n.backend, n, netConn, rawConn), nil
}
func (n *h1Node) _fetchTLS() (*H1Conn, error) {
	// TODO: dynamic address names?
	netConn, err := net.DialTimeout("tcp", n.address, n.backend.dialTimeout)
	if err != nil {
		n.markDown()
		return nil, err
	}
	if Debug() >= 2 {
		Printf("h1Node=%d dial %s OK!\n", n.id, n.address)
	}
	connID := n.backend.nextConnID()
	tlsConn := tls.Client(netConn, n.tlsConfig)
	if tlsConn.SetDeadline(time.Now().Add(10*time.Second)) != nil || tlsConn.Handshake() != nil {
		tlsConn.Close()
		return nil, err
	}
	n.IncSub(1)
	return getH1Conn(connID, n.backend, n, tlsConn, nil), nil
}
func (n *h1Node) _fetchUDS() (*H1Conn, error) {
	// TODO: dynamic address names?
	netConn, err := net.DialTimeout("unix", n.address, n.backend.dialTimeout)
	if err != nil {
		n.markDown()
		return nil, err
	}
	if Debug() >= 2 {
		Printf("h1Node=%d dial %s OK!\n", n.id, n.address)
	}
	connID := n.backend.nextConnID()
	rawConn, err := netConn.(*net.UnixConn).SyscallConn()
	if err != nil {
		netConn.Close()
		return nil, err
	}
	n.IncSub(1)
	return getH1Conn(connID, n.backend, n, netConn, rawConn), nil
}
func (n *h1Node) storeConn(h1Conn *H1Conn) {
	if h1Conn.isBroken() || n.isDown() || !h1Conn.isAlive() || !h1Conn.keepConn {
		if Debug() >= 2 {
			Printf("H1Conn[node=%d id=%d] closed\n", h1Conn.node.ID(), h1Conn.id)
		}
		n.closeConn(h1Conn)
	} else {
		if Debug() >= 2 {
			Printf("H1Conn[node=%d id=%d] pushed\n", h1Conn.node.ID(), h1Conn.id)
		}
		n.pushConn(h1Conn)
	}
}

func (n *h1Node) closeConn(h1Conn *H1Conn) {
	h1Conn.closeConn()
	putH1Conn(h1Conn)
	n.SubDone()
}

// poolH1Conn is the backend-side HTTP/1 connection pool.
var poolH1Conn sync.Pool

func getH1Conn(id int64, backend *H1Backend, node *h1Node, netConn net.Conn, rawConn syscall.RawConn) *H1Conn {
	var h1Conn *H1Conn
	if x := poolH1Conn.Get(); x == nil {
		h1Conn = new(H1Conn)
		stream := &h1Conn.stream
		req, resp := &stream.request, &stream.response
		req.shell = req
		req.stream = stream
		req.response = resp
		resp.shell = resp
		resp.stream = stream
	} else {
		h1Conn = x.(*H1Conn)
	}
	h1Conn.onGet(id, backend, node, netConn, rawConn)
	return h1Conn
}
func putH1Conn(h1Conn *H1Conn) {
	h1Conn.onPut()
	poolH1Conn.Put(h1Conn)
}

// H1Conn is the backend-side HTTP/1 connection.
type H1Conn struct {
	// Mixins
	webBackendConn_
	// Assocs
	stream H1Stream // an H1Conn has exactly one stream at a time, so just embed it
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	netConn  net.Conn        // the connection (TCP/TLS/UDS)
	rawConn  syscall.RawConn // used when netConn is TCP or UDS
	keepConn bool            // keep the connection after current stream? true by default
	// Conn states (zeros)
}

func (c *H1Conn) onGet(id int64, backend *H1Backend, node *h1Node, netConn net.Conn, rawConn syscall.RawConn) {
	c.webBackendConn_.onGet(id, backend, node)
	c.netConn = netConn
	c.rawConn = rawConn
	c.keepConn = true
}
func (c *H1Conn) onPut() {
	c.netConn = nil
	c.rawConn = nil
	c.webBackendConn_.onPut()
}

func (c *H1Conn) UseStream() *H1Stream {
	stream := &c.stream
	stream.onUse(c)
	return stream
}
func (c *H1Conn) EndStream(stream *H1Stream) {
	stream.onEnd()
}

func (c *H1Conn) Close() error { // only used by clients of dial
	netConn := c.netConn
	putH1Conn(c)
	return netConn.Close()
}

func (c *H1Conn) closeConn() { c.netConn.Close() } // used by codes which use fetch/store

// H1Stream is the backend-side HTTP/1 stream.
type H1Stream struct {
	// Mixins
	webBackendStream_
	// Assocs
	request  H1Request  // the backend-side http/1 request
	response H1Response // the backend-side http/1 response
	socket   *H1Socket  // the backend-side http/1 socket
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non zeros)
	conn *H1Conn // associated conn
	// Stream states (zeros)
}

func (s *H1Stream) onUse(conn *H1Conn) { // for non-zeros
	s.webBackendStream_.onUse()
	s.conn = conn
	s.request.onUse(Version1_1)
	s.response.onUse(Version1_1)
}
func (s *H1Stream) onEnd() { // for zeros
	s.response.onEnd()
	s.request.onEnd()
	s.socket = nil
	s.conn = nil
	s.webBackendStream_.onEnd()
}

func (s *H1Stream) webBroker() webBroker { return s.conn.Backend() }
func (s *H1Stream) webConn() webConn     { return s.conn }
func (s *H1Stream) remoteAddr() net.Addr { return s.conn.netConn.RemoteAddr() }

func (s *H1Stream) Request() *H1Request   { return &s.request }
func (s *H1Stream) Response() *H1Response { return &s.response }

func (s *H1Stream) ExecuteExchan() error { // request & response
	// TODO
	return nil
}
func (s *H1Stream) ReverseExchan(req Request, resp Response, bufferClientContent bool, bufferServerContent bool) error {
	// TODO
	return nil
}

func (s *H1Stream) ExecuteSocket() *H1Socket { // upgrade: websocket
	// TODO
	return s.socket
}

func (s *H1Stream) makeTempName(p []byte, unixTime int64) int {
	return s.conn.makeTempName(p, unixTime)
}

func (s *H1Stream) setWriteDeadline(deadline time.Time) error {
	conn := s.conn
	if deadline.Sub(conn.lastWrite) >= time.Second {
		if err := conn.netConn.SetWriteDeadline(deadline); err != nil {
			return err
		}
		conn.lastWrite = deadline
	}
	return nil
}
func (s *H1Stream) setReadDeadline(deadline time.Time) error {
	conn := s.conn
	if deadline.Sub(conn.lastRead) >= time.Second {
		if err := conn.netConn.SetReadDeadline(deadline); err != nil {
			return err
		}
		conn.lastRead = deadline
	}
	return nil
}

func (s *H1Stream) write(p []byte) (int, error)               { return s.conn.netConn.Write(p) }
func (s *H1Stream) writev(vector *net.Buffers) (int64, error) { return vector.WriteTo(s.conn.netConn) }
func (s *H1Stream) read(p []byte) (int, error)                { return s.conn.netConn.Read(p) }
func (s *H1Stream) readFull(p []byte) (int, error)            { return io.ReadFull(s.conn.netConn, p) }

func (s *H1Stream) isBroken() bool { return s.conn.isBroken() }
func (s *H1Stream) markBroken()    { s.conn.markBroken() }

// H1Request is the backend-side HTTP/1 request.
type H1Request struct { // outgoing. needs building
	// Mixins
	webBackendRequest_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *H1Request) setMethodURI(method []byte, uri []byte, hasContent bool) bool { // METHOD uri HTTP/1.1\r\n
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
func (r *H1Request) setAuthority(hostname []byte, colonPort []byte) bool { // used by proxies
	if r.stream.webConn().IsTLS() {
		if bytes.Equal(colonPort, bytesColonPort443) {
			colonPort = nil
		}
	} else if bytes.Equal(colonPort, bytesColonPort80) {
		colonPort = nil
	}
	headerSize := len(bytesHost) + len(bytesColonSpace) + len(hostname) + len(colonPort) + len(bytesCRLF) // host: xxx\r\n
	if from, _, ok := r._growFields(headerSize); ok {
		from += copy(r.fields[from:], bytesHost)
		r.fields[from] = ':'
		r.fields[from+1] = ' '
		from += 2
		from += copy(r.fields[from:], hostname)
		from += copy(r.fields[from:], colonPort)
		r._addCRLFHeader1(from)
		return true
	} else {
		return false
	}
}

func (r *H1Request) addHeader(name []byte, value []byte) bool   { return r.addHeader1(name, value) }
func (r *H1Request) header(name []byte) (value []byte, ok bool) { return r.header1(name) }
func (r *H1Request) hasHeader(name []byte) bool                 { return r.hasHeader1(name) }
func (r *H1Request) delHeader(name []byte) (deleted bool)       { return r.delHeader1(name) }
func (r *H1Request) delHeaderAt(i uint8)                        { r.delHeaderAt1(i) }

func (r *H1Request) AddCookie(name string, value string) bool {
	// TODO. need some space to place the cookie. use stream.unsafeMake()?
	return false
}
func (r *H1Request) copyCookies(req Request) bool { // used by proxies. merge into one "cookie" header
	headerSize := len(bytesCookie) + len(bytesColonSpace) // `cookie: `
	req.forCookies(func(cookie *pair, name []byte, value []byte) bool {
		headerSize += len(name) + 1 + len(value) + 2 // `name=value; `
		return true
	})
	if from, _, ok := r.growHeader(headerSize); ok {
		from += copy(r.fields[from:], bytesCookie)
		r.fields[from] = ':'
		r.fields[from+1] = ' '
		from += 2
		req.forCookies(func(cookie *pair, name []byte, value []byte) bool {
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

func (r *H1Request) sendChain() error { return r.sendChain1() }

func (r *H1Request) echoHeaders() error { return r.writeHeaders1() }
func (r *H1Request) echoChain() error   { return r.echoChain1(true) } // we always use HTTP/1.1 chunked

func (r *H1Request) addTrailer(name []byte, value []byte) bool   { return r.addTrailer1(name, value) }
func (r *H1Request) trailer(name []byte) (value []byte, ok bool) { return r.trailer1(name) }

func (r *H1Request) passHeaders() error       { return r.writeHeaders1() }
func (r *H1Request) passBytes(p []byte) error { return r.passBytes1(p) }

func (r *H1Request) finalizeHeaders() { // add at most 256 bytes
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
func (r *H1Request) finalizeVague() error { return r.finalizeVague1() }

func (r *H1Request) addedHeaders() []byte { return r.fields[r.controlEdge:r.fieldsEdge] }
func (r *H1Request) fixedHeaders() []byte { return http1BytesFixedRequestHeaders }

// H1Response is the backend-side HTTP/1 response.
type H1Response struct { // incoming. needs parsing
	// Mixins
	webBackendResponse_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *H1Response) recvHead() { // control + headers
	// The entire response head must be received within one timeout
	if err := r._beforeRead(&r.recvTime); err != nil {
		r.headResult = -1
		return
	}
	if !r.growHead1() { // r.input must be empty because we don't use pipelining in requests.
		// r.headResult is set.
		return
	}
	if !r.recvControl() || !r.recvHeaders1() || !r.examineHead() {
		// r.headResult is set.
		return
	}
	r.cleanInput()
	if Debug() >= 2 {
		Printf("[H1Stream=%d]<======= [%s]\n", r.stream.(*H1Stream).conn.id, r.input[r.head.from:r.head.edge])
	}
}
func (r *H1Response) recvControl() bool { // HTTP-version SP status-code SP [ reason-phrase ] CRLF
	// HTTP-version = HTTP-name "/" DIGIT "." DIGIT
	// HTTP-name = %x48.54.54.50 ; "HTTP", case-sensitive
	if have := r.inputEdge - r.pFore; have >= 9 {
		// r.pFore -> ' '
		// r.inputEdge -> after ' ' or more
		r.pFore += 8
	} else { // have < 9, but len("HTTP/1.X ") = 9.
		// r.pFore at 'H' -> ' '
		// r.inputEdge at "TTP/1.X " -> after ' '
		r.pFore = r.inputEdge - 1
		for i, n := int32(0), 9-have; i < n; i++ {
			if r.pFore++; r.pFore == r.inputEdge && !r.growHead1() {
				return false
			}
		}
	}
	if !bytes.Equal(r.input[r.pBack:r.pFore], bytesHTTP1_1) { // HTTP/1.0 is not supported in backend side
		r.headResult = StatusHTTPVersionNotSupported
		return false
	}

	// Skip SP
	if r.input[r.pFore] != ' ' {
		goto invalid
	}
	if r.pFore++; r.pFore == r.inputEdge && !r.growHead1() {
		return false
	}

	// status-code = 3DIGIT
	if b := r.input[r.pFore]; b >= '1' && b <= '9' {
		r.status = int16(b-'0') * 100
	} else {
		goto invalid
	}
	if r.pFore++; r.pFore == r.inputEdge && !r.growHead1() {
		return false
	}
	if b := r.input[r.pFore]; b >= '0' && b <= '9' {
		r.status += int16(b-'0') * 10
	} else {
		goto invalid
	}
	if r.pFore++; r.pFore == r.inputEdge && !r.growHead1() {
		return false
	}
	if b := r.input[r.pFore]; b >= '0' && b <= '9' {
		r.status += int16(b - '0')
	} else {
		goto invalid
	}
	if r.pFore++; r.pFore == r.inputEdge && !r.growHead1() {
		return false
	}

	// Skip SP
	if r.input[r.pFore] != ' ' {
		goto invalid
	}
	if r.pFore++; r.pFore == r.inputEdge && !r.growHead1() {
		return false
	}

	// reason-phrase = 1*( HTAB / SP / VCHAR / obs-text )
	for {
		if b := r.input[r.pFore]; b == '\n' {
			break
		}
		if r.pFore++; r.pFore == r.inputEdge && !r.growHead1() {
			return false
		}
	}
	r.receiving = webSectionHeaders
	// Skip '\n'
	if r.pFore++; r.pFore == r.inputEdge && !r.growHead1() {
		return false
	}
	return true
invalid:
	r.headResult, r.failReason = StatusBadRequest, "invalid character in control"
	return false
}
func (r *H1Response) cleanInput() {
	// r.pFore is at the beginning of content (if exists) or next response (if exists and is pipelined).
	if r.contentSize == -1 { // no content
		r.contentReceived = true
		if r.pFore < r.inputEdge { // still has data
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
	r.imme.set(r.pFore, r.inputEdge)
	if r.contentSize >= 0 { // sized mode
		if immeSize := int64(r.imme.size()); immeSize >= r.contentSize {
			r.contentReceived = true
			if immeSize > r.contentSize { // still has data
				// TODO: log? possible response splitting
			}
			r.receivedSize = r.contentSize
			r.contentText = r.input[r.pFore : r.pFore+int32(r.contentSize)] // exact.
			r.contentTextKind = webContentTextInput
		}
	} else { // vague mode
		// We don't know the size of vague content. Let chunked receivers to decide & clean r.input.
	}
}

func (r *H1Response) readContent() (p []byte, err error) { return r.readContent1() }

// poolH1Socket
var poolH1Socket sync.Pool

// H1Socket is the backend-side HTTP/1 websocket.
type H1Socket struct {
	// Mixins
	webBackendSocket_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}
