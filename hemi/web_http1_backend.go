// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// HTTP/1 backend implementation. See RFC 9112.

// Only HTTP/1.1 is used, so backends MUST support HTTP/1.1. Pipelining is not used.

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
	RegisterBackend("http1Backend", func(name string, stage *Stage) Backend {
		b := new(HTTP1Backend)
		b.onCreate(name, stage)
		return b
	})
}

// HTTP1Backend
type HTTP1Backend struct {
	// Parent
	Backend_[*http1Node]
	// Mixins
	_httpServend_
	// States
}

func (b *HTTP1Backend) onCreate(name string, stage *Stage) {
	b.Backend_.OnCreate(name, stage)
}

func (b *HTTP1Backend) OnConfigure() {
	b.Backend_.OnConfigure()
	b._httpServend_.onConfigure(b, 60*time.Second, 60*time.Second, 1000, TmpDir()+"/web/backends/"+b.name)

	// sub components
	b.ConfigureNodes()
}
func (b *HTTP1Backend) OnPrepare() {
	b.Backend_.OnPrepare()
	b._httpServend_.onPrepare(b)

	// sub components
	b.PrepareNodes()
}

func (b *HTTP1Backend) CreateNode(name string) Node {
	node := new(http1Node)
	node.onCreate(name, b)
	b.AddNode(node)
	return node
}

func (b *HTTP1Backend) FetchStream() (backendStream, error) {
	node := b.nodes[b.nextIndex()]
	return node.fetchStream()
}
func (b *HTTP1Backend) StoreStream(stream backendStream) {
	conn := stream.httpConn().(*backend1Conn)
	conn.node.storeStream(stream)
}

// http1Node is a node in HTTP1Backend.
type http1Node struct {
	// Parent
	Node_
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
	n.Node_.OnCreate(name)
	n.backend = backend
}

func (n *http1Node) OnConfigure() {
	n.Node_.OnConfigure()
	if n.tlsMode {
		n.tlsConfig.InsecureSkipVerify = true
		n.tlsConfig.NextProtos = []string{"http/1.1"}
	}
}
func (n *http1Node) OnPrepare() {
	n.Node_.OnPrepare()
}

func (n *http1Node) Maintain() { // runner
	n.Loop(time.Second, func(now time.Time) {
		// TODO: health check, markDown, markUp()
	})
	n.markDown()
	if size := n.closeFree(); size > 0 {
		n.SubsAddn(-size)
	}
	n.WaitSubs() // conns. TODO: max timeout?
	if DebugLevel() >= 2 {
		Printf("http1Node=%s done\n", n.name)
	}
	n.backend.DecSub()
}

func (n *http1Node) fetchStream() (backendStream, error) {
	conn := n.pullConn()
	down := n.isDown()
	if conn != nil {
		if conn.isAlive() && !conn.reachLimit() && !down {
			return conn.fetchStream()
		}
		n.closeConn(conn)
	}
	if down {
		return nil, errNodeDown
	}
	var err error
	if n.IsTLS() {
		conn, err = n._dialTLS()
	} else if n.IsUDS() {
		conn, err = n._dialUDS()
	} else {
		conn, err = n._dialTCP()
	}
	if err != nil {
		return nil, errNodeDown
	}
	n.IncSub()
	return conn.fetchStream()
}
func (n *http1Node) storeStream(stream backendStream) {
	conn := stream.(*backend1Stream).conn
	conn.storeStream(stream)

	if conn.isBroken() || n.isDown() || !conn.isAlive() || !conn.isPersistent() {
		n.closeConn(conn)
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

func (n *http1Node) _dialTLS() (*backend1Conn, error) {
	// TODO: dynamic address names?
	netConn, err := net.DialTimeout("tcp", n.address, n.backend.DialTimeout())
	if err != nil {
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
func (n *http1Node) _dialTCP() (*backend1Conn, error) {
	// TODO: dynamic address names?
	netConn, err := net.DialTimeout("tcp", n.address, n.backend.DialTimeout())
	if err != nil {
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

// poolBackend1Conn is the backend-side HTTP/1 connection pool.
var poolBackend1Conn sync.Pool

func getBackend1Conn(id int64, node *http1Node, netConn net.Conn, rawConn syscall.RawConn) *backend1Conn {
	var conn *backend1Conn
	if x := poolBackend1Conn.Get(); x == nil {
		conn = new(backend1Conn)
		stream := &conn.stream
		stream.conn = conn
		req, resp := &stream.request, &stream.response
		req.shell = req
		req.stream = stream
		req.response = resp
		resp.shell = resp
		resp.stream = stream
	} else {
		conn = x.(*backend1Conn)
	}
	conn.onGet(id, node, netConn, rawConn)
	return conn
}
func putBackend1Conn(conn *backend1Conn) {
	conn.onPut()
	poolBackend1Conn.Put(conn)
}

// backend1Conn is the backend-side HTTP/1 connection.
type backend1Conn struct {
	// Parent
	BackendConn_
	// Mixins
	_httpConn_
	// Assocs
	next   *backend1Conn  // the linked-list
	stream backend1Stream // an http/1 connection has exactly one stream
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	backend *HTTP1Backend
	node    *http1Node
	netConn net.Conn        // the connection (TCP/TLS/UDS)
	rawConn syscall.RawConn // used when netConn is TCP or UDS
	// Conn states (zeros)
}

func (c *backend1Conn) onGet(id int64, node *http1Node, netConn net.Conn, rawConn syscall.RawConn) {
	c.BackendConn_.OnGet(id, node.backend.aliveTimeout)
	c._httpConn_.onGet()

	c.backend = node.backend
	c.node = node
	c.netConn = netConn
	c.rawConn = rawConn
}
func (c *backend1Conn) onPut() {
	c.netConn = nil
	c.rawConn = nil
	c.node = nil
	c.backend = nil

	c._httpConn_.onPut()
	c.BackendConn_.OnPut()
}

func (c *backend1Conn) IsTLS() bool { return c.node.IsTLS() }
func (c *backend1Conn) IsUDS() bool { return c.node.IsUDS() }

func (c *backend1Conn) MakeTempName(p []byte, unixTime int64) int {
	return makeTempName(p, int64(c.backend.Stage().ID()), c.id, unixTime, c.counter.Add(1))
}

func (c *backend1Conn) HTTPBackend() HTTPBackend { return c.backend }

func (c *backend1Conn) reachLimit() bool {
	return c.usedStreams.Add(1) > c.HTTPBackend().MaxStreamsPerConn()
}

func (c *backend1Conn) fetchStream() (backendStream, error) {
	stream := &c.stream
	stream.onUse()
	return stream, nil
}
func (c *backend1Conn) storeStream(stream backendStream) {
	stream.(*backend1Stream).onEnd()
}

func (c *backend1Conn) Close() error {
	netConn := c.netConn
	putBackend1Conn(c)
	return netConn.Close()
}

// backend1Stream is the backend-side HTTP/1 stream.
type backend1Stream struct {
	// Mixins
	_httpStream_
	// Assocs
	conn     *backend1Conn    // the backend-side http/1 conn
	request  backend1Request  // the backend-side http/1 request
	response backend1Response // the backend-side http/1 response
	socket   *backend1Socket  // the backend-side http/1 websocket
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non zeros)
	// Stream states (zeros)
}

func (s *backend1Stream) onUse() { // for non-zeros
	s._httpStream_.onUse()

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

	s._httpStream_.onEnd()
}

func (s *backend1Stream) Request() backendRequest   { return &s.request }
func (s *backend1Stream) Response() backendResponse { return &s.response }
func (s *backend1Stream) Socket() backendSocket     { return nil } // TODO

func (s *backend1Stream) ExecuteExchan() error { // request & response
	// TODO
	return nil
}
func (s *backend1Stream) ExecuteSocket() error { // upgrade: websocket
	// TODO
	return nil
}

func (s *backend1Stream) httpServend() httpServend { return s.conn.HTTPBackend() }
func (s *backend1Stream) httpConn() httpConn       { return s.conn }
func (s *backend1Stream) remoteAddr() net.Addr     { return s.conn.netConn.RemoteAddr() }

func (s *backend1Stream) markBroken()    { s.conn.markBroken() }
func (s *backend1Stream) isBroken() bool { return s.conn.isBroken() }

func (s *backend1Stream) setWriteDeadline(deadline time.Time) error {
	conn := s.conn
	if deadline.Sub(conn.lastWrite) >= time.Second {
		if err := conn.netConn.SetWriteDeadline(deadline); err != nil {
			return err
		}
		conn.lastWrite = deadline
	}
	return nil
}
func (s *backend1Stream) setReadDeadline(deadline time.Time) error {
	conn := s.conn
	if deadline.Sub(conn.lastRead) >= time.Second {
		if err := conn.netConn.SetReadDeadline(deadline); err != nil {
			return err
		}
		conn.lastRead = deadline
	}
	return nil
}

func (s *backend1Stream) write(p []byte) (int, error) { return s.conn.netConn.Write(p) }
func (s *backend1Stream) writev(vector *net.Buffers) (int64, error) {
	return vector.WriteTo(s.conn.netConn)
}
func (s *backend1Stream) read(p []byte) (int, error)     { return s.conn.netConn.Read(p) }
func (s *backend1Stream) readFull(p []byte) (int, error) { return io.ReadFull(s.conn.netConn, p) }

// backend1Request is the backend-side HTTP/1 request.
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
func (r *backend1Request) setAuthority(hostname []byte, colonPort []byte) bool { // used by proxies
	if r.stream.httpConn().IsTLS() {
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

func (r *backend1Request) addHeader(name []byte, value []byte) bool   { return r.addHeader1(name, value) }
func (r *backend1Request) header(name []byte) (value []byte, ok bool) { return r.header1(name) }
func (r *backend1Request) hasHeader(name []byte) bool                 { return r.hasHeader1(name) }
func (r *backend1Request) delHeader(name []byte) (deleted bool)       { return r.delHeader1(name) }
func (r *backend1Request) delHeaderAt(i uint8)                        { r.delHeaderAt1(i) }

func (r *backend1Request) AddCookie(name string, value string) bool { // cookie: foo=bar; xyz=baz
	// TODO. need some space to place the cookie. use stream.unsafeMake()?
	return false
}
func (r *backend1Request) proxyCopyCookies(req Request) bool { // merge all cookies into one "cookie" header
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

func (r *backend1Request) sendChain() error { return r.sendChain1() }

func (r *backend1Request) echoHeaders() error { return r.writeHeaders1() }
func (r *backend1Request) echoChain() error   { return r.echoChain1(true) } // we always use HTTP/1.1 chunked

func (r *backend1Request) addTrailer(name []byte, value []byte) bool {
	return r.addTrailer1(name, value)
}
func (r *backend1Request) trailer(name []byte) (value []byte, ok bool) { return r.trailer1(name) }

func (r *backend1Request) passHeaders() error       { return r.writeHeaders1() }
func (r *backend1Request) passBytes(p []byte) error { return r.passBytes1(p) }

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

// backend1Response is the backend-side HTTP/1 response.
type backend1Response struct { // incoming. needs parsing
	// Parent
	backendResponse_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *backend1Response) recvHead() { // control + headers
	// The entire response head must be received within one timeout
	if err := r._beforeRead(&r.recvTime); err != nil {
		r.headResult = -1
		return
	}
	if !r.growHead1() { // r.input must be empty because we don't use pipelining in requests.
		// r.headResult is set.
		return
	}
	if !r._recvControl() || !r.recvHeaders1() || !r.examineHead() {
		// r.headResult is set.
		return
	}
	r.cleanInput()
	if DebugLevel() >= 2 {
		Printf("[backend1Stream=%d]<======= [%s]\n", r.stream.httpConn().ID(), r.input[r.head.from:r.head.edge])
	}
}
func (r *backend1Response) _recvControl() bool { // HTTP-version SP status-code SP [ reason-phrase ] CRLF
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
	r.receiving = httpSectionHeaders
	// Skip '\n'
	if r.pFore++; r.pFore == r.inputEdge && !r.growHead1() {
		return false
	}
	return true
invalid:
	r.headResult, r.failReason = StatusBadRequest, "invalid character in control"
	return false
}
func (r *backend1Response) cleanInput() {
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
			r.contentTextKind = httpContentTextInput
		}
	} else { // vague mode
		// We don't know the size of vague content. Let chunked receivers to decide & clean r.input.
	}
}

func (r *backend1Response) readContent() (p []byte, err error) { return r.readContent1() }

// poolBackend1Socket
var poolBackend1Socket sync.Pool

func getBackend1Socket(stream *backend1Stream) *backend1Socket {
	return nil
}
func putBackend1Socket(socket *backend1Socket) {
}

// backend1Socket is the backend-side HTTP/1 websocket.
type backend1Socket struct {
	// Parent
	backendSocket_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (s *backend1Socket) onUse() {
	s.backendSocket_.onUse()
}
func (s *backend1Socket) onEnd() {
	s.backendSocket_.onEnd()
}
