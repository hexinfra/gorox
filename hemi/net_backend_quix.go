// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// QUIX (UDP/UDS) backend.

package hemi

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/hexinfra/gorox/hemi/common/quic"
)

func init() {
	RegisterBackend("quixBackend", func(name string, stage *Stage) Backend {
		b := new(QUIXBackend)
		b.onCreate(name, stage)
		return b
	})
}

// QUIXBackend component.
type QUIXBackend struct {
	// Parent
	Backend_[*quixNode]
	// Mixins
	_streamHolder_
	// States
	health any // TODO
}

func (b *QUIXBackend) onCreate(name string, stage *Stage) {
	b.Backend_.OnCreate(name, stage)
}

func (b *QUIXBackend) OnConfigure() {
	b.Backend_.OnConfigure()
	b._streamHolder_.onConfigure(b, 1000)
}
func (b *QUIXBackend) OnPrepare() {
	b.Backend_.OnPrepare()
	b._streamHolder_.onPrepare(b)
}

func (b *QUIXBackend) CreateNode(name string) Node {
	node := new(quixNode)
	node.onCreate(name, b)
	b.AddNode(node)
	return node
}

func (b *QUIXBackend) Dial() (*QConn, error) {
	node := b.nodes[b.getNext()]
	return node.dial()
}

func (b *QUIXBackend) FetchConn() (*QConn, error) {
	return nil, nil
}
func (b *QUIXBackend) StoreConn(qConn *QConn) {
}

func (b *QUIXBackend) FetchStream() (*QStream, error) {
	return nil, nil
}
func (b *QUIXBackend) StoreStream(qStream *QStream) {
}

// quixNode is a node in QUIXBackend.
type quixNode struct {
	// Parent
	Node_
	// Assocs
	// States
}

func (n *quixNode) onCreate(name string, backend *QUIXBackend) {
	n.Node_.OnCreate(name, backend)
}

func (n *quixNode) OnConfigure() {
	n.Node_.OnConfigure()
}
func (n *quixNode) OnPrepare() {
	n.Node_.OnPrepare()
}

func (n *quixNode) Maintain() { // runner
	n.Loop(time.Second, func(now time.Time) {
		// TODO: health check
	})
	// TODO: wait for all conns
	if Debug() >= 2 {
		Printf("quixNode=%s done\n", n.name)
	}
	n.backend.DecSub()
}

func (n *quixNode) dial() (*QConn, error) {
	// TODO
	return nil, nil
}

func (n *quixNode) fetchConn() (*QConn, error) {
	// Note: A QConn can be used concurrently, limited by maxStreams.
	// TODO
	return nil, nil
}
func (n *quixNode) storeConn(qConn *QConn) {
	// Note: A QConn can be used concurrently, limited by maxStreams.
	// TODO
}

func (n *quixNode) fetchStream() (*QStream, error) {
	return nil, nil
}
func (n *quixNode) storeStream(qStream *QStream) {
}

// poolQConn
var poolQConn sync.Pool

func getQConn(id int64, node *quixNode, quicConn *quic.Conn) *QConn {
	var qConn *QConn
	if x := poolQConn.Get(); x == nil {
		qConn = new(QConn)
	} else {
		qConn = x.(*QConn)
	}
	qConn.onGet(id, node, quicConn)
	return qConn
}
func putQConn(qConn *QConn) {
	qConn.onPut()
	poolQConn.Put(qConn)
}

// QConn is a backend-side quix connection to quixNode.
type QConn struct {
	// Parent
	BackendConn_
	// Conn states (non-zeros)
	quicConn   *quic.Conn
	maxStreams int32 // how many streams are allowed on this connection?
	// Conn states (zeros)
	usedStreams atomic.Int32 // how many streams have been used?
	broken      atomic.Bool  // is connection broken?
}

func (c *QConn) onGet(id int64, node *quixNode, quicConn *quic.Conn) {
	c.BackendConn_.OnGet(id, node)
	c.quicConn = quicConn
	c.maxStreams = node.Backend().(*QUIXBackend).MaxStreamsPerConn()
}
func (c *QConn) onPut() {
	c.quicConn = nil
	c.usedStreams.Store(0)
	c.broken.Store(false)
	c.BackendConn_.OnPut()
}

func (c *QConn) reachLimit() bool { return c.usedStreams.Add(1) > c.maxStreams }

func (c *QConn) isBroken() bool { return c.broken.Load() }
func (c *QConn) markBroken()    { c.broken.Store(true) }

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

// poolQStream
var poolQStream sync.Pool

func getQStream(conn *QConn, quicStream *quic.Stream) *QStream {
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

// QStream is a bidirectional stream of QConn.
type QStream struct {
	// TODO
	conn       *QConn
	quicStream *quic.Stream
}

func (s *QStream) onUse(conn *QConn, quicStream *quic.Stream) {
	s.conn = conn
	s.quicStream = quicStream
}
func (s *QStream) onEnd() {
	s.conn = nil
	s.quicStream = nil
}

func (s *QStream) Write(p []byte) (n int, err error) {
	// TODO
	return
}
func (s *QStream) Read(p []byte) (n int, err error) {
	// TODO
	return
}

func (s *QStream) Close() error {
	// TODO
	return nil
}
