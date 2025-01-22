// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// QUIX (QUIC over UDP/UDS) backend implementation. See RFC 8999, RFC 9000, RFC 9001, and RFC 9002.

package hemi

import (
	"sync"
	"time"

	"github.com/hexinfra/gorox/hemi/library/tcp2"
)

func init() {
	RegisterBackend("quixBackend", func(compName string, stage *Stage) Backend {
		b := new(QUIXBackend)
		b.onCreate(compName, stage)
		return b
	})
}

// QUIXBackend component.
type QUIXBackend struct {
	// Parent
	Backend_[*quixNode]
	// States
}

func (b *QUIXBackend) onCreate(compName string, stage *Stage) {
	b.Backend_.OnCreate(compName, stage)
}

func (b *QUIXBackend) OnConfigure() {
	b.Backend_.OnConfigure()

	// sub components
	b.ConfigureNodes()
}
func (b *QUIXBackend) OnPrepare() {
	b.Backend_.OnPrepare()

	// sub components
	b.PrepareNodes()
}

func (b *QUIXBackend) CreateNode(compName string) Node {
	node := new(quixNode)
	node.onCreate(compName, b.stage, b)
	b.AddNode(node)
	return node
}

func (b *QUIXBackend) Dial() (*QConn, error) {
	node := b.nodes[b.nodeIndexGet()]
	return node.dial()
}

func (b *QUIXBackend) FetchStream() (*QStream, error) {
	// TODO
	return nil, nil
}
func (b *QUIXBackend) StoreStream(qStream *QStream) {
	// TODO
}

// quixNode is a node in QUIXBackend.
type quixNode struct {
	// Parent
	Node_[*QUIXBackend]
	// Mixins
	_quixHolder_
	// States
}

func (n *quixNode) onCreate(compName string, stage *Stage, backend *QUIXBackend) {
	n.Node_.OnCreate(compName, stage, backend)
}

func (n *quixNode) OnConfigure() {
	n.Node_.OnConfigure()
	n._quixHolder_.onConfigure(n)
}
func (n *quixNode) OnPrepare() {
	n.Node_.OnPrepare()
	n._quixHolder_.onPrepare(n)
}

func (n *quixNode) Maintain() { // runner
	n.LoopRun(time.Second, func(now time.Time) {
		// TODO: health check, markDown, markUp()
	})
	// TODO: wait for all conns
	if DebugLevel() >= 2 {
		Printf("quixNode=%s done\n", n.compName)
	}
	n.backend.DecSub() // node
}

func (n *quixNode) dial() (*QConn, error) {
	// TODO. note: use n.IncSubConns()?
	return nil, nil
}

func (n *quixNode) fetchStream() (*QStream, error) {
	// Note: A QConn can be used concurrently, limited by maxConcurrentStreams.
	// TODO
	return nil, nil
}
func (n *quixNode) storeStream(qStream *QStream) {
	// Note: A QConn can be used concurrently, limited by maxConcurrentStreams.
	// TODO
}

// QConn is a backend-side quix connection to quixNode.
type QConn struct {
	// Parent
	quixConn_
	// Conn states (non-zeros)
	node *quixNode
	// Conn states (zeros)
}

var poolQConn sync.Pool

func getQConn(id int64, node *quixNode, quicConn *tcp2.Conn) *QConn {
	var conn *QConn
	if x := poolQConn.Get(); x == nil {
		conn = new(QConn)
	} else {
		conn = x.(*QConn)
	}
	conn.onGet(id, node, quicConn)
	return conn
}
func putQConn(conn *QConn) {
	conn.onPut()
	poolQConn.Put(conn)
}

func (c *QConn) onGet(id int64, node *quixNode, quicConn *tcp2.Conn) {
	c.quixConn_.onGet(id, node.Stage(), quicConn, node.UDSMode(), node.TLSMode(), node.MaxCumulativeStreamsPerConn(), node.MaxConcurrentStreamsPerConn())

	c.node = node
}
func (c *QConn) onPut() {
	c.node = nil

	c.quixConn_.onPut()
}

func (c *QConn) ranOut() bool { return c.cumulativeStreams.Add(1) > c.maxCumulativeStreams }
func (c *QConn) FetchStream() (*QStream, error) {
	// TODO
	return nil, nil
}
func (c *QConn) StoreStream(stream *QStream) {
	// TODO
}

func (c *QConn) Close() error {
	quicConn := c.quicConn
	putQConn(c)
	return quicConn.Close()
}

// QStream is a bidirectional stream of QConn.
type QStream struct {
	// Parent
	quixStream_
	// Assocs
	conn *QConn
	// Stream states (non-zeros)
	// Stream states (zeros)
}

var poolQStream sync.Pool

func getQStream(conn *QConn, quicStream *tcp2.Stream) *QStream {
	var stream *QStream
	if x := poolQStream.Get(); x == nil {
		stream = new(QStream)
	} else {
		stream = x.(*QStream)
	}
	stream.onUse(conn, quicStream)
	return stream
}
func putQStream(stream *QStream) {
	stream.onEnd()
	poolQStream.Put(stream)
}

func (s *QStream) onUse(conn *QConn, quicStream *tcp2.Stream) {
	s.quixStream_.onUse(quicStream)
	s.conn = conn
}
func (s *QStream) onEnd() {
	s.conn = nil
	s.quixStream_.onEnd()
}

func (s *QStream) Write(src []byte) (n int, err error) {
	// TODO
	return
}
func (s *QStream) Read(dst []byte) (n int, err error) {
	// TODO
	return
}

func (s *QStream) Close() error {
	// TODO
	return nil
}
