// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// HTTP/1.x backend implementation. See RFC 9112.

// For HTTP/1.x backends, only HTTP/1.1 is used, so HTTP/1.x backends MUST support HTTP/1.1. Pipelining is not used.

package hemi

import (
	"bytes"
	"crypto/tls"
	"net"
	"sync"
	"syscall"
	"time"
)

func init() {
	RegisterBackend("http1Backend", func(compName string, stage *Stage) Backend {
		b := new(HTTP1Backend)
		b.onCreate(compName, stage)
		return b
	})
}

// HTTP1Backend
type HTTP1Backend struct {
	// Parent
	Backend_[*http1Node]
	// States
}

func (b *HTTP1Backend) onCreate(compName string, stage *Stage) {
	b.Backend_.OnCreate(compName, stage)
}

func (b *HTTP1Backend) OnConfigure() {
	b.Backend_.OnConfigure()
	b.ConfigureNodes()
}
func (b *HTTP1Backend) OnPrepare() {
	b.Backend_.OnPrepare()
	b.PrepareNodes()
}

func (b *HTTP1Backend) CreateNode(compName string) Node {
	node := new(http1Node)
	node.onCreate(compName, b.stage, b)
	b.AddNode(node)
	return node
}

func (b *HTTP1Backend) FetchStream(servReq ServerRequest) (BackendStream, error) {
	return b.nodes[b.nodeIndexGet()].fetchStream()
}
func (b *HTTP1Backend) StoreStream(backStream BackendStream) {
	backStream1 := backStream.(*backend1Stream)
	backStream1.conn.node.storeStream(backStream1)
}

// http1Node is a node in HTTP1Backend.
type http1Node struct {
	// Parent
	httpNode_[*HTTP1Backend]
	// States
	connPool struct {
		sync.Mutex
		head *backend1Conn
		tail *backend1Conn
		qnty int
	}
}

func (n *http1Node) onCreate(compName string, stage *Stage, backend *HTTP1Backend) {
	n.httpNode_.onCreate(compName, stage, backend)
}

func (n *http1Node) OnConfigure() {
	n.httpNode_.onConfigure()
	if n.tlsMode {
		n.tlsConfig.InsecureSkipVerify = true
		n.tlsConfig.NextProtos = []string{"http/1.1"}
	}
}
func (n *http1Node) OnPrepare() {
	n.httpNode_.onPrepare()
}

func (n *http1Node) Maintain() { // runner
	n.LoopRun(time.Second, func(now time.Time) {
		// TODO: health check, markDown, markUp()
	})
	n.markDown()
	if size := n.closeIdle(); size > 0 {
		n.DecSubConns(size)
	}
	n.WaitSubConns() // TODO: max timeout?
	if DebugLevel() >= 2 {
		Printf("http1Node=%s done\n", n.compName)
	}
	n.backend.DecSub() // node
}

func (n *http1Node) fetchStream() (*backend1Stream, error) {
	backConn := n.pullConn()
	nodeDown := n.isDown()
	if backConn != nil {
		if !nodeDown && backConn.isAlive() && backConn.cumulativeStreams.Add(1) <= n.maxCumulativeStreamsPerConn {
			return backConn.fetchStream()
		}
		backConn.Close()
		n.DecSubConn()
	}
	if nodeDown {
		return nil, errNodeDown
	}
	var err error
	if n.UDSMode() {
		backConn, err = n._dialUDS()
	} else if n.TLSMode() {
		backConn, err = n._dialTLS()
	} else {
		backConn, err = n._dialTCP()
	}
	if err != nil {
		return nil, errNodeDown
	}
	n.IncSubConn()
	return backConn.fetchStream()
}
func (n *http1Node) _dialUDS() (*backend1Conn, error) {
	// TODO: dynamic address names?
	netConn, err := net.DialTimeout("unix", n.address, n.DialTimeout())
	if err != nil {
		n.markDown()
		return nil, err
	}
	if DebugLevel() >= 2 {
		Printf("http1Node=%s dial %s OK!\n", n.compName, n.address)
	}
	connID := n.nextConnID()
	rawConn, err := netConn.(*net.UnixConn).SyscallConn()
	if err != nil {
		netConn.Close()
		return nil, err
	}
	return getBackend1Conn(connID, n, netConn, rawConn), nil
}
func (n *http1Node) _dialTLS() (*backend1Conn, error) {
	// TODO: dynamic address names?
	netConn, err := net.DialTimeout("tcp", n.address, n.DialTimeout())
	if err != nil {
		// TODO: handle ephemeral port exhaustion
		n.markDown()
		return nil, err
	}
	if DebugLevel() >= 2 {
		Printf("http1Node=%s dial %s OK!\n", n.compName, n.address)
	}
	connID := n.nextConnID()
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
	netConn, err := net.DialTimeout("tcp", n.address, n.DialTimeout())
	if err != nil {
		// TODO: handle ephemeral port exhaustion
		n.markDown()
		return nil, err
	}
	if DebugLevel() >= 2 {
		Printf("http1Node=%s dial %s OK!\n", n.compName, n.address)
	}
	connID := n.nextConnID()
	rawConn, err := netConn.(*net.TCPConn).SyscallConn()
	if err != nil {
		netConn.Close()
		return nil, err
	}
	return getBackend1Conn(connID, n, netConn, rawConn), nil
}
func (n *http1Node) storeStream(backStream *backend1Stream) {
	backConn := backStream.conn
	backConn.storeStream(backStream)

	if !n.isDown() && !backConn.isBroken() && backConn.isAlive() && backConn.persistent {
		if DebugLevel() >= 2 {
			Printf("Backend1Conn[node=%s id=%d] pushed\n", n.CompName(), backConn.id)
		}
		backConn.expireTime = time.Now().Add(n.idleTimeout)
		n.pushConn(backConn)
	} else {
		if DebugLevel() >= 2 {
			Printf("Backend1Conn[node=%s id=%d] closed\n", n.CompName(), backConn.id)
		}
		backConn.Close()
		n.DecSubConn()
	}
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
func (n *http1Node) closeIdle() int {
	list := &n.connPool

	list.Lock()
	defer list.Unlock()

	conn := list.head
	for conn != nil {
		next := conn.next
		conn.next = nil
		conn.Close()
		conn = next
	}
	qnty := list.qnty
	list.qnty = 0
	list.head = nil
	list.tail = nil

	return qnty
}

// backend1Conn is the backend-side HTTP/1.x connection.
type backend1Conn struct {
	// Parent
	http1Conn_
	// Mixins
	_backendConn_[*http1Node]
	// Assocs
	next   *backend1Conn  // the linked-list
	stream backend1Stream // an http/1.x connection has exactly one stream
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	// Conn states (zeros)
}

var poolBackend1Conn sync.Pool

func getBackend1Conn(id int64, node *http1Node, netConn net.Conn, rawConn syscall.RawConn) *backend1Conn {
	var backConn *backend1Conn
	if x := poolBackend1Conn.Get(); x == nil {
		backConn = new(backend1Conn)
		backStream := &backConn.stream
		backResp, backReq := &backStream.response, &backStream.request
		backResp.stream = backStream
		backResp.in = backResp
		backReq.stream = backStream
		backReq.out = backReq
		backReq.response = backResp
	} else {
		backConn = x.(*backend1Conn)
	}
	backConn.onGet(id, node, netConn, rawConn)
	return backConn
}
func putBackend1Conn(backConn *backend1Conn) {
	backConn.onPut()
	poolBackend1Conn.Put(backConn)
}

func (c *backend1Conn) onGet(id int64, node *http1Node, netConn net.Conn, rawConn syscall.RawConn) {
	c.http1Conn_.onGet(id, node, netConn, rawConn)
	c._backendConn_.onGet(node)
}
func (c *backend1Conn) onPut() {
	c._backendConn_.onPut()
	c.node = nil // put here due to Go's limitation
	c.http1Conn_.onPut()
}

func (c *backend1Conn) fetchStream() (*backend1Stream, error) {
	backStream := &c.stream
	backStream.onUse(c.nextStreamID(), c)
	return backStream, nil
}
func (c *backend1Conn) storeStream(backStream *backend1Stream) {
	backStream.onEnd()
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
	// Mixins
	_backendStream_
	// Assocs
	response backend1Response // the backend-side http/1.x response
	request  backend1Request  // the backend-side http/1.x request
	socket   *backend1Socket  // the backend-side http/1.x webSocket
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (s *backend1Stream) onUse(id int64, conn *backend1Conn) { // for non-zeros
	s.http1Stream_.onUse(id, conn)
	s._backendStream_.onUse()

	s.response.onUse()
	s.request.onUse()
}
func (s *backend1Stream) onEnd() { // for zeros
	s.request.onEnd()
	s.response.onEnd()
	if s.socket != nil {
		s.socket.onEnd()
		s.socket = nil
	}

	s._backendStream_.onEnd()
	s.http1Stream_.onEnd()
	s.conn = nil // we can't do this in http1Stream_.onEnd() due to Go's limit, so put here
}

func (s *backend1Stream) Response() BackendResponse { return &s.response }
func (s *backend1Stream) Request() BackendRequest   { return &s.request }
func (s *backend1Stream) Socket() BackendSocket     { return nil } // TODO. See RFC 6455

// backend1Response is the backend-side HTTP/1.x response.
type backend1Response struct { // incoming. needs parsing
	// Parent
	backendResponse_
	// Assocs
	in1 _http1In_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *backend1Response) onUse() {
	r.backendResponse_.onUse(Version1_1)
	r.in1.onUse(&r._httpIn_)
}
func (r *backend1Response) onEnd() {
	r.backendResponse_.onEnd()
	r.in1.onEnd()
}

func (r *backend1Response) recvHead() { // control data + header section
	// The entire response head must be received within one read timeout
	if err := r.stream.setReadDeadline(); err != nil {
		r.headResult = -1
		return
	}
	if !r.in1.growHead() { // r.input must be empty because we don't use pipelining in requests.
		// r.headResult is set.
		return
	}
	if !r._recvControlData() || !r.in1.recvHeaderLines() || !r.examineHead() {
		// r.headResult is set.
		return
	}
	r.tidyInput()
	if DebugLevel() >= 2 {
		Printf("[backend1Stream=%d]=======> [%s]\n", r.stream.ID(), r.input[r.head.from:r.head.edge])
	}
}
func (r *backend1Response) _recvControlData() bool { // status-line = HTTP-version SP status-code SP [ reason-phrase ] CRLF
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
			if r.elemFore++; r.elemFore == r.inputEdge && !r.in1.growHead() {
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
	if r.elemFore++; r.elemFore == r.inputEdge && !r.in1.growHead() {
		return false
	}

	// status-code = 3DIGIT
	if b := r.input[r.elemFore]; b >= '1' && b <= '9' {
		r.status = int16(b-'0') * 100
	} else {
		goto invalid
	}
	if r.elemFore++; r.elemFore == r.inputEdge && !r.in1.growHead() {
		return false
	}
	if b := r.input[r.elemFore]; b >= '0' && b <= '9' {
		r.status += int16(b-'0') * 10
	} else {
		goto invalid
	}
	if r.elemFore++; r.elemFore == r.inputEdge && !r.in1.growHead() {
		return false
	}
	if b := r.input[r.elemFore]; b >= '0' && b <= '9' {
		r.status += int16(b - '0')
	} else {
		goto invalid
	}
	if r.elemFore++; r.elemFore == r.inputEdge && !r.in1.growHead() {
		return false
	}

	// Skip SP
	if r.input[r.elemFore] != ' ' {
		goto invalid
	}
	if r.elemFore++; r.elemFore == r.inputEdge && !r.in1.growHead() {
		return false
	}

	// reason-phrase = 1*( HTAB / SP / VCHAR / obs-text )
	for {
		if b := r.input[r.elemFore]; b == '\n' {
			break
		}
		if r.elemFore++; r.elemFore == r.inputEdge && !r.in1.growHead() {
			return false
		}
	}
	r.receiving = httpSectionHeaders
	// Skip '\n'
	if r.elemFore++; r.elemFore == r.inputEdge && !r.in1.growHead() {
		return false
	}
	return true
invalid:
	r.headResult, r.failReason = StatusBadRequest, "invalid character in control"
	return false
}
func (r *backend1Response) tidyInput() {
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

func (r *backend1Response) readContent() (data []byte, err error) { return r.in1.readContent() }

// backend1Request is the backend-side HTTP/1.x request.
type backend1Request struct { // outgoing. needs building
	// Parent
	backendRequest_
	// Assocs
	out1 _http1Out_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *backend1Request) onUse() {
	r.backendRequest_.onUse(Version1_1)
	r.out1.onUse(&r._httpOut_)
}
func (r *backend1Request) onEnd() {
	r.backendRequest_.onEnd()
	r.out1.onEnd()
}

func (r *backend1Request) addHeader(name []byte, value []byte) bool {
	return r.out1.addHeader(name, value)
}
func (r *backend1Request) header(name []byte) (value []byte, ok bool) { return r.out1.header(name) }
func (r *backend1Request) hasHeader(name []byte) bool                 { return r.out1.hasHeader(name) }
func (r *backend1Request) delHeader(name []byte) (deleted bool)       { return r.out1.delHeader(name) }
func (r *backend1Request) delHeaderAt(i uint8)                        { r.out1.delHeaderAt(i) }

func (r *backend1Request) AddCookie(name string, value string) bool { // cookie: foo=bar; xyz=baz
	// TODO. need some space to place the cookie. use stream.unsafeMake()?
	return false
}
func (r *backend1Request) proxyCopyCookies(servReq ServerRequest) bool { // NOTE: merge all cookies into one "cookie" header field
	headerSize := len(bytesCookie) + len(bytesColonSpace) // `cookie: `
	servReq.proxyWalkCookies(func(cookie *pair, cookieName []byte, cookieValue []byte) bool {
		headerSize += len(cookieName) + 1 + len(cookieValue) + 2 // `name=value; `
		return true
	})
	if from, _, ok := r.growHeaders(headerSize); ok {
		from += copy(r.fields[from:], bytesCookie)
		r.fields[from] = ':'
		r.fields[from+1] = ' '
		from += 2
		servReq.proxyWalkCookies(func(cookie *pair, cookieName []byte, cookieValue []byte) bool {
			from += copy(r.fields[from:], cookieName)
			r.fields[from] = '='
			from++
			from += copy(r.fields[from:], cookieValue)
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

func (r *backend1Request) sendChain() error { return r.out1.sendChain() }

func (r *backend1Request) echoHeaders() error { return r.out1.writeHeaders() }
func (r *backend1Request) echoChain() error   { return r.out1.echoChain(true) } // we always use HTTP/1.1 chunked

func (r *backend1Request) addTrailer(name []byte, value []byte) bool {
	return r.out1.addTrailer(name, value)
}
func (r *backend1Request) trailer(name []byte) (value []byte, ok bool) { return r.out1.trailer(name) }

func (r *backend1Request) proxySetMethodURI(method []byte, uri []byte, hasContent bool) bool { // METHOD uri HTTP/1.1\r\n
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
	if r.stream.TLSMode() {
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
		r.out1._addCRLFHeader(from)
		return true
	} else {
		return false
	}
}

func (r *backend1Request) proxyPassHeaders() error          { return r.out1.writeHeaders() }
func (r *backend1Request) proxyPassBytes(data []byte) error { return r.out1.proxyPassBytes(data) }

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
				r.out1._addFixedHeader(bytesContentLength, sizeBuffer[:n])
			}
		}
		// content-type: application/octet-stream\r\n
		if r.iContentType == 0 {
			r.fieldsEdge += uint16(copy(r.fields[r.fieldsEdge:], http1BytesContentTypeStream))
		}
	}
	if r.addTETrailers {
		r.fieldsEdge += uint16(copy(r.fields[r.fieldsEdge:], http1BytesTETrailers))
		r.fieldsEdge += uint16(copy(r.fields[r.fieldsEdge:], http1BytesConnectionAliveTE))
	} else {
		// connection: keep-alive\r\n
		r.fieldsEdge += uint16(copy(r.fields[r.fieldsEdge:], http1BytesConnectionKeepAlive))
	}
}
func (r *backend1Request) finalizeVague() error { return r.out1.finalizeVague() } // we always use http/1.1 in the backend side.

func (r *backend1Request) addedHeaders() []byte { return r.fields[r.controlEdge:r.fieldsEdge] }
func (r *backend1Request) fixedHeaders() []byte { return http1BytesFixedRequestHeaders }

// backend1Socket is the backend-side HTTP/1.x webSocket.
type backend1Socket struct { // incoming and outgoing
	// Parent
	backendSocket_
	// Assocs
	so1 _http1Socket_
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
	s.so1.onUse(&s._httpSocket_)
}
func (s *backend1Socket) onEnd() {
	s.backendSocket_.onEnd()
	s.so1.onEnd()
}

func (s *backend1Socket) backendTodo1() {
	s.backendTodo()
	s.so1.todo1()
}
