// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HTTP/1 client implementation.

// Only HTTP/1.1 is used. For simplicity, HTTP/1.1 pipelining is not used.

package internal

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"sync"
	"syscall"
	"time"
)

func init() {
	registerBackend("http1Backend", func(name string, stage *Stage) backend {
		b := new(HTTP1Backend)
		b.init(name, stage)
		return b
	})
}

// HTTP1Backend
type HTTP1Backend struct {
	// Mixins
	httpBackend_
	// States
	nodes []*http1Node
}

func (b *HTTP1Backend) init(name string, stage *Stage) {
	b.httpBackend_.init(name, stage)
}

func (b *HTTP1Backend) OnConfigure() {
	b.configure()
	// nodes
	v, ok := b.Find("nodes")
	if !ok {
		UseExitln("nodes is required for backends")
	}
	vNodes, ok := v.List()
	if !ok {
		UseExitln("bad nodes")
	}
	for id, elem := range vNodes {
		vNode, ok := elem.Dict()
		if !ok {
			UseExitln("node in nodes must be a dict")
		}
		node := new(http1Node)
		node.init(int32(id), b)
		// address
		vAddress, ok := vNode["address"]
		if !ok {
			UseExitln("address is required in node")
		}
		if address, ok := vAddress.String(); ok && address != "" {
			node.address = address
		}
		// weight
		vWeight, ok := vNode["weight"]
		if ok {
			if weight, ok := vWeight.Int32(); ok && weight > 0 {
				node.weight = weight
			} else {
				UseExitln("bad weight in node")
			}
		} else {
			node.weight = 1
		}
		// keepConns
		vKeepConns, ok := vNode["keepConns"]
		if ok {
			if keepConns, ok := vKeepConns.Int32(); ok && keepConns > 0 {
				node.keepConns = keepConns
			} else {
				UseExitln("bad keepConns in node")
			}
		} else {
			node.keepConns = 10
		}
		b.nodes = append(b.nodes, node)
	}
}
func (b *HTTP1Backend) OnPrepare() {
	b.prepare(len(b.nodes))
}
func (b *HTTP1Backend) OnShutdown() {
	b.shutdown()
}

func (b *HTTP1Backend) maintain() { // blocking
	for _, node := range b.nodes {
		node.checkHealth()
		time.Sleep(time.Second)
	}
}

func (b *HTTP1Backend) FetchConn() (*H1Conn, error) {
	node := b.nodes[b.getIndex()]
	return node.fetchConn()
}
func (b *HTTP1Backend) StoreConn(conn *H1Conn) {
	conn.node.storeConn(conn)
}

// http1Node is a node in HTTP1Backend.
type http1Node struct {
	// Mixins
	httpNode_
	// Assocs
	backend *HTTP1Backend
	// States
}

func (n *http1Node) init(id int32, backend *HTTP1Backend) {
	n.httpNode_.init(id)
	n.backend = backend
}

func (n *http1Node) checkHealth() {
	// TODO
	for {
		time.Sleep(time.Second)
	}
}

func (n *http1Node) fetchConn() (*H1Conn, error) {
	conn := n.takeConn()
	down := n.isDown()
	if conn != nil {
		hConn := conn.(*H1Conn)
		if hConn.isAlive() && !hConn.reachLimit() && !down {
			return hConn, nil
		}
		hConn.closeConn()
		putH1Conn(hConn)
	}
	if down {
		return nil, errNodeDown
	}

	netConn, err := net.DialTimeout("tcp", n.address, n.backend.dialTimeout)
	if err != nil {
		n.markDown()
		return nil, err
	}
	connID := n.backend.nextConnID()
	if n.backend.tlsMode {
		tlsConn := tls.Client(netConn, n.backend.tlsConfig)
		// TODO: timeout
		if err := tlsConn.Handshake(); err != nil {
			tlsConn.Close()
			return nil, err
		}
		return getH1Conn(connID, n.backend, n, tlsConn, nil), nil
	} else {
		rawConn, err := netConn.(*net.TCPConn).SyscallConn()
		if err != nil {
			netConn.Close()
			return nil, err
		}
		return getH1Conn(connID, n.backend, n, netConn, rawConn), nil
	}
}
func (n *http1Node) storeConn(hConn *H1Conn) {
	if hConn.isBroken() || n.isDown() || !hConn.isAlive() {
		if IsDevel() {
			fmt.Printf("H1Conn[node=%d id=%d] closed\n", hConn.node.id, hConn.id)
		}
		hConn.closeConn()
		putH1Conn(hConn)
	} else {
		if IsDevel() {
			fmt.Printf("H1Conn[node=%d id=%d] pushed\n", hConn.node.id, hConn.id)
		}
		n.pushConn(hConn)
	}
}

// poolH1Conn is the client-side HTTP/1 connection pool.
var poolH1Conn sync.Pool

func getH1Conn(id int64, client httpClient, node *http1Node, netConn net.Conn, rawConn syscall.RawConn) *H1Conn {
	var conn *H1Conn
	if x := poolH1Conn.Get(); x == nil {
		conn = new(H1Conn)
		stream := &conn.stream
		req, resp := &stream.request, &stream.response
		req.shell = req
		req.stream = stream
		req.response = resp
		resp.shell = resp
		resp.stream = stream
	} else {
		conn = x.(*H1Conn)
	}
	conn.onGet(id, client, node, netConn, rawConn)
	return conn
}
func putH1Conn(conn *H1Conn) {
	conn.onPut()
	poolH1Conn.Put(conn)
}

// H1Conn is the client-side HTTP/1 connection.
type H1Conn struct {
	// Mixins
	hConn_
	// Assocs
	stream H1Stream // an H1Conn has exactly one stream at a time, so just embed it
	// Conn states (buffers)
	// Conn states (controlled)
	// Conn states (non-zeros)
	node    *http1Node      // belonging node if client is http backend
	netConn net.Conn        // the connection (TCP/TLS)
	rawConn syscall.RawConn // used when netConn is TCP
	// Conn states (zeros)
}

func (c *H1Conn) onGet(id int64, client httpClient, node *http1Node, netConn net.Conn, rawConn syscall.RawConn) {
	c.hConn_.onGet(id, client)
	c.node = node
	c.netConn = netConn
	c.rawConn = rawConn
}
func (c *H1Conn) onPut() {
	c.node = nil
	c.netConn = nil
	c.rawConn = nil
	c.hConn_.onPut()
}

func (c *H1Conn) Stream() *H1Stream { return &c.stream }

func (c *H1Conn) closeConn() { c.netConn.Close() }

// H1Stream is the client-side HTTP/1 stream.
type H1Stream struct {
	// Mixins
	hStream_
	// Assocs
	request  H1Request  // the client-side http/1 request
	response H1Response // the client-side http/1 response
	// Stream states (buffers)
	// Stream states (controlled)
	// Stream states (non zeros)
	conn *H1Conn // belonging conn
	// Stream states (zeros)
}

func (s *H1Stream) onUse(conn *H1Conn) { // for non-zeros
	s.hStream_.onUse()
	s.conn = conn
	s.request.onUse()
	s.response.onUse()
}
func (s *H1Stream) onEnd() { // for zeros
	s.response.onEnd()
	s.request.onEnd()
	s.conn = nil
	s.hStream_.onEnd()
}

func (s *H1Stream) Request() *H1Request   { return &s.request }
func (s *H1Stream) Response() *H1Response { return &s.response }
func (s *H1Stream) Socket() *H1Socket     { return nil } // TODO

func (s *H1Stream) forwardProxy(req Request, resp Response, bufferClientContent bool, bufferServerContent bool) error {
	// TODO
	return nil
}
func (s *H1Stream) reverseProxy(req Request, resp Response, bufferClientContent bool, bufferServerContent bool) error {
	// TODO
	return nil
}

func (s *H1Stream) makeTempName(p []byte, seconds int64) (from int, edge int) {
	return s.conn.makeTempName(p, seconds)
}
func (s *H1Stream) peerAddr() net.Addr {
	return s.conn.netConn.RemoteAddr()
}

func (s *H1Stream) setWriteDeadline(deadline time.Time) error {
	conn := s.conn
	if deadline.Sub(conn.lastWrite) >= conn.client.WriteTimeout()/4 {
		if err := conn.netConn.SetWriteDeadline(deadline); err != nil {
			return err
		}
		conn.lastWrite = deadline
	}
	return nil
}
func (s *H1Stream) setReadDeadline(deadline time.Time) error {
	conn := s.conn
	if deadline.Sub(conn.lastRead) >= conn.client.ReadTimeout()/4 {
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

func (s *H1Stream) getHolder() holder { return s.conn.getClient() }

// H1Request is the client-side HTTP/1 request.
type H1Request struct {
	// Mixins
	hRequest_
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *H1Request) setControl(method []byte, uri []byte) bool {
	size := len(method) + 1 + len(uri) + 1 + len(httpBytesHTTP1_1) + len(httpBytesCRLF) // METHOD uri HTTP/1.1\r\n
	if from, edge, ok := r._growFields(size); ok {
		r.controlEdge = uint16(edge)
		from += copy(r.fields[from:], method)
		r.fields[from] = ' '
		from++
		from += copy(r.fields[from:], uri)
		r.fields[from] = ' '
		from++
		from += copy(r.fields[from:], httpBytesHTTP1_1) // we always use HTTP/1.1
		r.fields[from] = '\r'
		r.fields[from+1] = '\n'
		return true
	} else {
		return false
	}
}
func (r *H1Request) control() []byte { return r.fields[0:r.controlEdge] }

func (r *H1Request) header(name []byte) (value []byte, ok bool) {
	return r.header1(name)
}
func (r *H1Request) addHeader(name []byte, value []byte) bool {
	return r.addHeader1(name, value)
}
func (r *H1Request) delHeader(name []byte) (deleted bool) {
	return r.delHeader1(name)
}
func (r *H1Request) addedHeaders() []byte {
	return r.fields[r.controlEdge:r.fieldsEdge]
}
func (r *H1Request) fixedHeaders() []byte {
	return http1BytesFixedRequestHeaders
}

func (r *H1Request) doSend(chain Chain) error {
	r.finalizeHeaders()
	var vector [][]byte                        // waiting for write
	if nBlocks := chain.Size(); nBlocks == 1 { // content chain has exactly one block
		vector = r.fixedVector[0:4]
	} else { // nBlocks >= 2
		vector = make([][]byte, 3+nBlocks) // TODO(diogin): get from pool? defer pool.put()
	}
	return r.doSend1(chain, vector)
}

func (r *H1Request) pushHeaders() error {
	return r.pushHeaders1()
}
func (r *H1Request) doPush(chain Chain) error {
	return r.doPush1(chain, true)
}
func (r *H1Request) addTrailer(name []byte, value []byte) bool {
	return r.addTrailer1(name, value)
}
func (r *H1Request) pushEnd() error {
	return r.pushEnd1()
}

func (r *H1Request) finalizeHeaders() { // add at most 256 bytes. not used by proxies!
	if r.contentSize != -1 {
		if r.contentSize != -2 { // content-length: 12345
			lengthBuffer := r.stream.smallStack() // 64 bytes is enough for length
			from, edge := i64ToDec(r.contentSize, lengthBuffer)
			r._addFixedHeader1(httpBytesContentLength, lengthBuffer[from:edge])
		} else { // transfer-encoding: chunked
			r.fieldsEdge += uint16(copy(r.fields[r.fieldsEdge:], http1BytesTransferChunked))
		}
		// content-type: text/html; charset=utf-8
		if !r.contentTypeSet {
			r.fieldsEdge += uint16(copy(r.fields[r.fieldsEdge:], http1BytesContentTypeTextHTML))
		}
	}
	// connection: keep-alive
	r.fieldsEdge += uint16(copy(r.fields[r.fieldsEdge:], http1BytesConnectionKeepAlive))
}

// H1Response is the client-side HTTP/1 response.
type H1Response struct {
	// Mixins
	hResponse_
	// Assocs
	// Stream states (non-zeros)
}

func (r *H1Response) recvHead() { // control + headers
	// The entire response head must be received within one timeout
	if err := r._prepareRead(&r.receiveTime); err != nil {
		r.headResult = -1
		return
	}
	if !r._growHead1() || !r._recvControl() || !r._recvHeaders1() || !r.checkHead() {
		// r.headResult is set.
		return
	}
	r._cleanInput()
	if IsDevel() {
		fmt.Printf("<------- [%s]\n", r.input[r.head.from:r.head.edge])
	}
}
func (r *H1Response) _recvControl() bool { // HTTP-version SP status-code SP [ reason-phrase ] CRLF
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
			if r.pFore++; r.pFore == r.inputEdge && !r._growHead1() {
				return false
			}
		}
	}
	if version := r.input[r.pBack:r.pFore]; bytes.Equal(version, httpBytesHTTP1_1) {
		r.versionCode = Version1_1
	} else if bytes.Equal(version, httpBytesHTTP1_0) {
		r.versionCode = Version1_0
	} else {
		r.headResult = StatusHTTPVersionNotSupported
		return false
	}

	if r.input[r.pFore] != ' ' {
		r.headResult, r.headReason = StatusBadRequest, "invalid space"
	}
	if r.pFore++; r.pFore == r.inputEdge && !r._growHead1() {
		return false
	}

	// status-code = 3DIGIT
	if b := r.input[r.pFore]; b >= '1' && b <= '9' {
		r.status = int16(b-'0') * 100
	} else {
		r.headResult, r.headReason = StatusBadRequest, "invalid character in status"
		return false
	}
	if r.pFore++; r.pFore == r.inputEdge && !r._growHead1() {
		return false
	}

	if b := r.input[r.pFore]; b >= '0' && b <= '9' {
		r.status += int16(b-'0') * 10
	} else {
		r.headResult, r.headReason = StatusBadRequest, "invalid character in status"
		return false
	}
	if r.pFore++; r.pFore == r.inputEdge && !r._growHead1() {
		return false
	}

	if b := r.input[r.pFore]; b >= '0' && b <= '9' {
		r.status += int16(b - '0')
	} else {
		r.headResult, r.headReason = StatusBadRequest, "invalid character in status"
		return false
	}
	if r.pFore++; r.pFore == r.inputEdge && !r._growHead1() {
		return false
	}

	if r.input[r.pFore] != ' ' {
		r.headResult, r.headReason = StatusBadRequest, "invalid character in status"
		return false
	}
	if r.pFore++; r.pFore == r.inputEdge && !r._growHead1() {
		return false
	}

	// reason-phrase = 1*( HTAB / SP / VCHAR / obs-text )
	for {
		if b := r.input[r.pFore]; b == '\n' {
			break
		}
		if r.pFore++; r.pFore == r.inputEdge && !r._growHead1() {
			return false
		}
	}
	r.receiving = httpSectionHeaders
	// Skip '\n'
	if r.pFore++; r.pFore == r.inputEdge && !r._growHead1() {
		return false
	}

	return true
}
func (r *H1Response) _cleanInput() {
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
	// content exists (identity or chunked)
	r.imme.set(r.pFore, r.inputEdge)
	if r.contentSize >= 0 { // identity mode
		if immeSize := int64(r.imme.size()); immeSize >= r.contentSize {
			r.contentReceived = true
			if immeSize > r.contentSize { // still has data
				// TODO: log? possible response splitting
			}
			r.sizeReceived = r.contentSize
			r.contentBlob = r.input[r.pFore : r.pFore+int32(r.contentSize)] // exact.
			r.contentBlobKind = httpContentBlobInput
		}
	} else { // chunked mode
		// We don't know the length of chunked content. Let chunked receivers to decide & clean r.input.
	}
}

func (r *H1Response) readContent() (from int, edge int, err error) {
	return r.readContent1()
}

// H1Socket is the client-side HTTP/1 websocket.
type H1Socket struct {
	// Mixins
	hSocket_
	// Stream states (zeros)
}

func (s *H1Socket) onUse() {
	s.hSocket_.onUse()
}
func (s *H1Socket) onEnd() {
	s.hSocket_.onEnd()
}
