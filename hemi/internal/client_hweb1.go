// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HWEB/1 client implementation.

// HWEB/1 is a binary HTTP/1.1 without WebSocket, TCP Tunnel, and UDP Tunnel support.

package internal

import (
	"io"
	"net"
	"sync"
	"syscall"
	"time"
)

func init() {
	registerFixture(signHWEB1Outgate)
	RegisterBackend("hweb1Backend", func(name string, stage *Stage) Backend {
		b := new(HWEB1Backend)
		b.onCreate(name, stage)
		return b
	})
}

const signHWEB1Outgate = "hweb1Outgate"

func createHWEB1Outgate(stage *Stage) *HWEB1Outgate {
	hweb1 := new(HWEB1Outgate)
	hweb1.onCreate(stage)
	hweb1.setShell(hweb1)
	return hweb1
}

// HWEB1Outgate
type HWEB1Outgate struct {
	// Mixins
	webOutgate_
	// States
	conns any // TODO
}

func (f *HWEB1Outgate) onCreate(stage *Stage) {
	f.webOutgate_.onCreate(signHWEB1Outgate, stage)
}

func (f *HWEB1Outgate) OnConfigure() {
	f.webOutgate_.onConfigure(f)
}
func (f *HWEB1Outgate) OnPrepare() {
	f.webOutgate_.onPrepare(f)
}

func (f *HWEB1Outgate) run() { // goroutine
	f.Loop(time.Second, func(now time.Time) {
		// TODO
	})
	if IsDebug(2) {
		Debugln("hweb1Outgate done")
	}
	f.stage.SubDone()
}

func (f *HWEB1Outgate) Dial(address string, tlsMode bool) (*B1Conn, error) {
	netConn, err := net.DialTimeout("tcp", address, f.dialTimeout)
	if err != nil {
		return nil, err
	}
	connID := f.nextConnID()
	tcpConn := netConn.(*net.TCPConn)
	rawConn, err := tcpConn.SyscallConn()
	if err != nil {
		tcpConn.Close()
		return nil, err
	}
	return getB1Conn(connID, f, nil, tcpConn, rawConn), nil
}

// HWEB1Backend
type HWEB1Backend struct {
	// Mixins
	webBackend_[*hweb1Node]
	// States
}

func (b *HWEB1Backend) onCreate(name string, stage *Stage) {
	b.webBackend_.onCreate(name, stage, b)
}

func (b *HWEB1Backend) OnConfigure() {
	b.webBackend_.onConfigure(b)
}
func (b *HWEB1Backend) OnPrepare() {
	b.webBackend_.onPrepare(b, len(b.nodes))
}

func (b *HWEB1Backend) createNode(id int32) *hweb1Node {
	node := new(hweb1Node)
	node.init(id, b)
	return node
}

func (b *HWEB1Backend) FetchConn() (*B1Conn, error) {
	node := b.nodes[b.getNext()]
	return node.fetchConn()
}
func (b *HWEB1Backend) StoreConn(conn *B1Conn) {
	conn.node.storeConn(conn)
}

// hweb1Node is a node in HWEB1Backend.
type hweb1Node struct {
	// Mixins
	webNode_
	// Assocs
	backend *HWEB1Backend
	// States
}

func (n *hweb1Node) init(id int32, backend *HWEB1Backend) {
	n.webNode_.init(id)
	n.backend = backend
}

func (n *hweb1Node) Maintain() { // goroutine
	n.Loop(time.Second, func(now time.Time) {
		// TODO: health check
	})
	n.markDown()
	if size := n.closeFree(); size > 0 {
		n.IncSub(0 - size)
	}
	n.WaitSubs() // conns
	if IsDebug(2) {
		Debugf("hweb1Node=%d done\n", n.id)
	}
	n.backend.SubDone()
}

func (n *hweb1Node) fetchConn() (*B1Conn, error) {
	conn := n.pullConn()
	down := n.isDown()
	if conn != nil {
		b1Conn := conn.(*B1Conn)
		if b1Conn.isAlive() && !b1Conn.reachLimit() && !down {
			return b1Conn, nil
		}
		n.closeConn(b1Conn)
	}
	if down {
		return nil, errNodeDown
	}

	netConn, err := net.DialTimeout("tcp", n.address, n.backend.dialTimeout)
	if err != nil {
		n.markDown()
		return nil, err
	}
	if IsDebug(2) {
		Debugf("hweb1Node=%d dial %s OK!\n", n.id, n.address)
	}
	connID := n.backend.nextConnID()
	tcpConn := netConn.(*net.TCPConn)
	rawConn, err := tcpConn.SyscallConn()
	if err != nil {
		netConn.Close()
		return nil, err
	}
	n.IncSub(1)
	return getB1Conn(connID, n.backend, n, tcpConn, rawConn), nil
}
func (n *hweb1Node) storeConn(b1Conn *B1Conn) {
	if b1Conn.isBroken() || n.isDown() || !b1Conn.isAlive() || !b1Conn.keepConn {
		if IsDebug(2) {
			Debugf("B1Conn[node=%d id=%d] closed\n", b1Conn.node.id, b1Conn.id)
		}
		n.closeConn(b1Conn)
	} else {
		if IsDebug(2) {
			Debugf("B1Conn[node=%d id=%d] pushed\n", b1Conn.node.id, b1Conn.id)
		}
		n.pushConn(b1Conn)
	}
}

func (n *hweb1Node) closeConn(b1Conn *B1Conn) {
	b1Conn.closeConn()
	putB1Conn(b1Conn)
	n.SubDone()
}

// poolB1Conn is the client-side HWEB/1 connection pool.
var poolB1Conn sync.Pool

func getB1Conn(id int64, client webClient, node *hweb1Node, tcpConn *net.TCPConn, rawConn syscall.RawConn) *B1Conn {
	var conn *B1Conn
	if x := poolB1Conn.Get(); x == nil {
		conn = new(B1Conn)
		stream := &conn.stream
		req, resp := &stream.request, &stream.response
		req.shell = req
		req.stream = stream
		req.response = resp
		resp.shell = resp
		resp.stream = stream
	} else {
		conn = x.(*B1Conn)
	}
	conn.onGet(id, client, node, tcpConn, rawConn)
	return conn
}
func putB1Conn(conn *B1Conn) {
	conn.onPut()
	poolB1Conn.Put(conn)
}

// B1Conn is the client-side HWEB/1 connection.
type B1Conn struct {
	// Mixins
	clientConn_
	// Assocs
	stream B1Stream // an B1Conn has exactly one stream at a time, so just embed it
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	node     *hweb1Node      // associated node if client is hweb1 backend
	tcpConn  *net.TCPConn    // the connection
	rawConn  syscall.RawConn // ...
	keepConn bool            // keep the connection after current stream? true by default
	// Conn states (zeros)
}

func (c *B1Conn) onGet(id int64, client webClient, node *hweb1Node, tcpConn *net.TCPConn, rawConn syscall.RawConn) {
	c.clientConn_.onGet(id, client)
	c.node = node
	c.tcpConn = tcpConn
	c.rawConn = rawConn
	c.keepConn = true
}
func (c *B1Conn) onPut() {
	c.node = nil
	c.tcpConn = nil
	c.rawConn = nil
	c.clientConn_.onPut()
}

func (c *B1Conn) UseStream() *B1Stream {
	stream := &c.stream
	stream.onUse(c)
	return stream
}
func (c *B1Conn) EndStream(stream *B1Stream) {
	stream.onEnd()
}

func (c *B1Conn) Close() error { // only used by clients of dial
	tcpConn := c.tcpConn
	putB1Conn(c)
	return tcpConn.Close()
}

func (c *B1Conn) closeConn() { c.tcpConn.Close() } // used by codes other than dial

// B1Stream is the client-side HWEB/1 stream.
type B1Stream struct {
	// Mixins
	webStream_
	// Assocs
	request  B1Request  // the client-side hweb/1 request
	response B1Response // the client-side hweb/1 response
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non zeros)
	conn *B1Conn // associated conn
	// Stream states (zeros)
}

func (s *B1Stream) onUse(conn *B1Conn) { // for non-zeros
	s.webStream_.onUse()
	s.conn = conn
	s.request.onUse(Version1_1)
	s.response.onUse(Version1_1)
}
func (s *B1Stream) onEnd() { // for zeros
	s.response.onEnd()
	s.request.onEnd()
	s.conn = nil
	s.webStream_.onEnd()
}

func (s *B1Stream) keeper() webKeeper  { return s.conn.getClient() }
func (s *B1Stream) peerAddr() net.Addr { return s.conn.tcpConn.RemoteAddr() }

func (s *B1Stream) Request() *B1Request   { return &s.request }
func (s *B1Stream) Response() *B1Response { return &s.response }

func (s *B1Stream) ExecuteNormal() error { // request & response
	// TODO
	return nil
}

func (s *B1Stream) ForwardProxy(req Request, resp Response, bufferClientContent bool, bufferServerContent bool) error {
	// TODO
	return nil
}
func (s *B1Stream) ReverseProxy(req Request, resp Response, bufferClientContent bool, bufferServerContent bool) error {
	// TODO
	return nil
}

func (s *B1Stream) makeTempName(p []byte, unixTime int64) (from int, edge int) {
	return s.conn.makeTempName(p, unixTime)
}

func (s *B1Stream) setWriteDeadline(deadline time.Time) error {
	conn := s.conn
	if deadline.Sub(conn.lastWrite) >= time.Second {
		if err := conn.tcpConn.SetWriteDeadline(deadline); err != nil {
			return err
		}
		conn.lastWrite = deadline
	}
	return nil
}
func (s *B1Stream) setReadDeadline(deadline time.Time) error {
	conn := s.conn
	if deadline.Sub(conn.lastRead) >= time.Second {
		if err := conn.tcpConn.SetReadDeadline(deadline); err != nil {
			return err
		}
		conn.lastRead = deadline
	}
	return nil
}

func (s *B1Stream) write(p []byte) (int, error)               { return s.conn.tcpConn.Write(p) }
func (s *B1Stream) writev(vector *net.Buffers) (int64, error) { return vector.WriteTo(s.conn.tcpConn) }
func (s *B1Stream) read(p []byte) (int, error)                { return s.conn.tcpConn.Read(p) }
func (s *B1Stream) readFull(p []byte) (int, error)            { return io.ReadFull(s.conn.tcpConn, p) }

func (s *B1Stream) isBroken() bool { return s.conn.isBroken() }
func (s *B1Stream) markBroken()    { s.conn.markBroken() }

// B1Request is the client-side HWEB/1 request.
type B1Request struct { // outgoing. needs building
	// Mixins
	clientRequest_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *B1Request) setMethodURI(method []byte, uri []byte, hasContent bool) bool {
	return false
}
func (r *B1Request) setAuthority(hostname []byte, colonPort []byte) bool {
	return false
}

func (r *B1Request) addHeader(name []byte, value []byte) bool   { return r.addHeaderB1(name, value) }
func (r *B1Request) header(name []byte) (value []byte, ok bool) { return r.headerB1(name) }
func (r *B1Request) hasHeader(name []byte) bool                 { return r.hasHeaderB1(name) }
func (r *B1Request) delHeader(name []byte) (deleted bool)       { return r.delHeaderB1(name) }
func (r *B1Request) delHeaderAt(o uint8)                        { r.delHeaderAtB1(o) }

func (r *B1Request) AddCookie(name string, value string) bool {
	// TODO. need some space to place the cookie
	return false
}
func (r *B1Request) copyCookies(req Request) bool {
	return false
}

func (r *B1Request) sendChain() error { return r.sendChainB1() }

func (r *B1Request) echoHeaders() error { return r.writeHeadersB1() }
func (r *B1Request) echoChain() error   { return r.echoChainB1() }

func (r *B1Request) addTrailer(name []byte, value []byte) bool   { return r.addTrailerB1(name, value) }
func (r *B1Request) trailer(name []byte) (value []byte, ok bool) { return r.trailerB1(name) }

func (r *B1Request) passHeaders() error       { return r.writeHeadersB1() }
func (r *B1Request) passBytes(p []byte) error { return r.passBytesB1(p) }

func (r *B1Request) finalizeHeaders() { // add at most 256 bytes
}
func (r *B1Request) finalizeUnsized() error { return r.finalizeUnsizedB1() }

func (r *B1Request) addedHeaders() []byte { return nil }
func (r *B1Request) fixedHeaders() []byte { return nil }

// B1Response is the client-side HWEB/1 response.
type B1Response struct { // incoming. needs parsing
	// Mixins
	clientResponse_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *B1Response) recvHead() {
}
func (r *B1Response) cleanInput() {}

func (r *B1Response) readContent() (p []byte, err error) { return r.readContentB1() }
