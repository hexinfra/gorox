// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HWEB client implementation.

package internal

import (
	"io"
	"net"
	"sync"
	"syscall"
	"time"
)

// hwebBackend
type hwebBackend struct {
	// Mixins
	webBackend_[*hwebNode]
}

func (b *hwebBackend) onCreate(name string, stage *Stage) {
	b.webBackend_.onCreate(name, stage, b)
}

func (b *hwebBackend) OnConfigure() {
	b.webBackend_.onConfigure(b)
}
func (b *hwebBackend) OnPrepare() {
	b.webBackend_.onPrepare(b, len(b.nodes))
}

func (b *hwebBackend) createNode(id int32) *hwebNode {
	node := new(hwebNode)
	node.init(id, b)
	return node
}

func (b *hwebBackend) FetchConn() (*hConn, error) {
	node := b.nodes[b.getNext()]
	return node.fetchConn()
}
func (b *hwebBackend) StoreConn(conn *hConn) {
	conn.node.storeConn(conn)
}

// hwebNode
type hwebNode struct {
	// Mixins
	webNode_
	// Assocs
	backend *hwebBackend
	// States
}

func (n *hwebNode) init(id int32, backend *hwebBackend) {
	n.webNode_.init(id)
	n.backend = backend
}

func (n *hwebNode) maintain(shut chan struct{}) { // goroutine
	Loop(time.Second, shut, func(now time.Time) {
		// TODO: health check
	})
	// TODO: wait for all conns
	if IsDebug(2) {
		Debugf("http2Node=%d done\n", n.id)
	}
	n.backend.SubDone()
}

func (n *hwebNode) fetchConn() (*hConn, error) {
	// Note: An hConn can be used concurrently, limited by maxStreams.
	// TODO
	return nil, nil
}
func (n *hwebNode) storeConn(wConn *hConn) {
	// Note: An hConn can be used concurrently, limited by maxStreams.
	// TODO
}

// poolHConn is the client-side HWEB connection pool.
var poolHConn sync.Pool

func getHConn(id int64, client webClient, node *hwebNode, tcpConn *net.TCPConn, rawConn syscall.RawConn) *hConn {
	var conn *hConn
	if x := poolHConn.Get(); x == nil {
		conn = new(hConn)
	} else {
		conn = x.(*hConn)
	}
	conn.onGet(id, client, node, tcpConn, rawConn)
	return conn
}
func putHConn(conn *hConn) {
	conn.onPut()
	poolHConn.Put(conn)
}

// hConn
type hConn struct {
	// Mixins
	wConn_
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	node    *hwebNode    // associated node
	tcpConn *net.TCPConn // the connection
	rawConn syscall.RawConn
	// Conn states (zeros)
	activeStreams int32 // concurrent streams
}

func (c *hConn) onGet(id int64, client webClient, node *hwebNode, tcpConn *net.TCPConn, rawConn syscall.RawConn) {
	c.wConn_.onGet(id, client)
	c.node = node
	c.tcpConn = tcpConn
	c.rawConn = rawConn
}
func (c *hConn) onPut() {
	c.wConn_.onPut()
	c.node = nil
	c.tcpConn = nil
	c.rawConn = nil
	c.activeStreams = 0
}

func (c *hConn) FetchStream() *hStream {
	// TODO: stream.onUse()
	return nil
}
func (c *hConn) StoreStream(stream *hStream) {
	// TODO
	stream.onEnd()
}

func (c *hConn) Close() error { // only used by clients of dial
	// TODO
	return nil
}

func (c *hConn) setWriteDeadline(deadline time.Time) error {
	if deadline.Sub(c.lastWrite) >= time.Second {
		if err := c.tcpConn.SetWriteDeadline(deadline); err != nil {
			return err
		}
		c.lastWrite = deadline
	}
	return nil
}
func (c *hConn) setReadDeadline(deadline time.Time) error {
	if deadline.Sub(c.lastRead) >= time.Second {
		if err := c.tcpConn.SetReadDeadline(deadline); err != nil {
			return err
		}
		c.lastRead = deadline
	}
	return nil
}

func (c *hConn) write(p []byte) (int, error) { return c.tcpConn.Write(p) }
func (c *hConn) writev(vector *net.Buffers) (int64, error) {
	// Will consume vector automatically
	return vector.WriteTo(c.tcpConn)
}
func (c *hConn) readAtLeast(p []byte, n int) (int, error) {
	return io.ReadAtLeast(c.tcpConn, p, n)
}

func (c *hConn) closeConn() { c.tcpConn.Close() } // used by codes other than dial

// poolHStream
var poolHStream sync.Pool

func getHStream(conn *hConn, id uint32) *hStream {
	return nil
}
func putHStream(stream *hStream) {
}

// hStream
type hStream struct {
	// Mixins
	wStream_
	// Assocs
	request  hRequest
	response hResponse
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	conn *hConn
	id   uint32
	// Stream states (zeros)
	hStream0 // all values must be zero by default in this struct!
}
type hStream0 struct { // for fast reset, entirely
}

// hRequest is the client-side HWEB request.
type hRequest struct { // outgoing. needs building
	// Mixins
	wRequest_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *hRequest) setMethodURI(method []byte, uri []byte, hasContent bool) bool { // :method = method, :uri = uri
	// TODO: set :method and :uri
	return false
}
func (r *hRequest) setAuthority(hostname []byte, colonPort []byte) bool { // used by agents
	// TODO: set :authority
	return false
}

func (r *hRequest) addHeader(name []byte, value []byte) bool   { return r.addHeaderH(name, value) }
func (r *hRequest) header(name []byte) (value []byte, ok bool) { return r.headerH(name) }
func (r *hRequest) hasHeader(name []byte) bool                 { return r.hasHeaderH(name) }
func (r *hRequest) delHeader(name []byte) (deleted bool)       { return r.delHeaderH(name) }
func (r *hRequest) delHeaderAt(o uint8)                        { r.delHeaderAtH(o) }

func (r *hRequest) addedHeaders() []byte { return nil }
func (r *hRequest) fixedHeaders() []byte { return nil }

// hResponse is the client-side HWEB response.
type hResponse struct { // incoming. needs parsing
	// Mixins
	wResponse_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

func (r *hResponse) readContent() (p []byte, err error) { return r.readContentH() }
