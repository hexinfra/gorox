// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// QUIC (UDP/UDS) network client implementation.

package internal

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/hexinfra/gorox/hemi/common/quix"
)

func init() {
	RegisterBackend("quicBackend", func(name string, stage *Stage) Backend {
		b := new(QUICBackend)
		b.onCreate(name, stage)
		return b
	})
}

// QUICBackend component.
type QUICBackend struct {
	// Mixins
	Backend_[*quicNode]
	streamHolder_
	loadBalancer_
	// States
	health any // TODO
}

func (b *QUICBackend) onCreate(name string, stage *Stage) {
	b.Backend_.onCreate(name, stage, b)
	b.loadBalancer_.init()
}

func (b *QUICBackend) OnConfigure() {
	b.Backend_.onConfigure()
	b.streamHolder_.onConfigure(b, 1000)
	b.loadBalancer_.onConfigure(b)
}
func (b *QUICBackend) OnPrepare() {
	b.Backend_.onPrepare()
	b.streamHolder_.onPrepare(b)
	b.loadBalancer_.onPrepare(len(b.nodes))
}

func (b *QUICBackend) createNode(id int32) *quicNode {
	node := new(quicNode)
	node.init(id, b)
	return node
}

func (b *QUICBackend) Dial() (*QConn, error) {
	// TODO
	return nil, nil
}
func (b *QUICBackend) FetchConn() (*QConn, error) {
	// TODO
	return nil, nil
}
func (b *QUICBackend) StoreConn(qConn *QConn) {
	// TODO
}

// quicNode is a node in QUICBackend.
type quicNode struct {
	// Mixins
	Node_
	// Assocs
	backend *QUICBackend
	// States
}

func (n *quicNode) init(id int32, backend *QUICBackend) {
	n.Node_.init(id)
	n.backend = backend
}

func (n *quicNode) Maintain() { // runner
	n.Loop(time.Second, func(now time.Time) {
		// TODO: health check
	})
	// TODO: wait for all conns
	if Debug() >= 2 {
		Printf("quicNode=%d done\n", n.id)
	}
	n.backend.SubDone()
}

func (n *quicNode) dial() (*QConn, error) {
	// TODO
	return nil, nil
}
func (n *quicNode) fetchConn() (*QConn, error) {
	// Note: A QConn can be used concurrently, limited by maxStreams.
	// TODO
	return nil, nil
}
func (n *quicNode) storeConn(qConn *QConn) {
	// Note: A QConn can be used concurrently, limited by maxStreams.
	// TODO
}

// poolQConn
var poolQConn sync.Pool

func getQConn(id int64, udsMode bool, backend *QUICBackend, node *quicNode, quixConn *quix.Conn) *QConn {
	var conn *QConn
	if x := poolQConn.Get(); x == nil {
		conn = new(QConn)
	} else {
		conn = x.(*QConn)
	}
	conn.onGet(id, udsMode, backend, node, quixConn)
	return conn
}
func putQConn(conn *QConn) {
	conn.onPut()
	poolQConn.Put(conn)
}

// QConn is a client-side quic connection to quicNode.
type QConn struct {
	// Mixins
	Conn_
	// Conn states (non-zeros)
	backend    *QUICBackend
	node       *quicNode
	quixConn   *quix.Conn
	maxStreams int32 // how many streams are allowed on this connection?
	// Conn states (zeros)
	usedStreams atomic.Int32 // how many streams has been used?
	broken      atomic.Bool  // is connection broken?
}

func (c *QConn) onGet(id int64, udsMode bool, backend *QUICBackend, node *quicNode, quixConn *quix.Conn) {
	c.Conn_.onGet(id, udsMode, true, time.Now().Add(backend.AliveTimeout()))
	c.backend = backend
	c.node = node
	c.quixConn = quixConn
	c.maxStreams = backend.MaxStreamsPerConn()
}
func (c *QConn) onPut() {
	c.Conn_.onPut()
	c.backend = nil
	c.node = nil
	c.quixConn = nil
	c.usedStreams.Store(0)
	c.broken.Store(false)
}

func (c *QConn) Backend() *QUICBackend { return c.backend }

func (c *QConn) reachLimit() bool {
	return c.usedStreams.Add(1) > c.maxStreams
}

func (c *QConn) isBroken() bool { return c.broken.Load() }
func (c *QConn) markBroken()    { c.broken.Store(true) }

func (c *QConn) FetchStream() *QStream {
	// TODO
	return nil
}
func (c *QConn) StoreStream(stream *QStream) {
	// TODO
}

// poolQStream
var poolQStream sync.Pool

func getQStream(conn *QConn, quixStream *quix.Stream) *QStream {
	var stream *QStream
	if x := poolQStream.Get(); x == nil {
		stream = new(QStream)
	} else {
		stream = x.(*QStream)
	}
	stream.onUse(conn, quixStream)
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
	quixStream *quix.Stream
}

func (s *QStream) onUse(conn *QConn, quixStream *quix.Stream) {
	s.conn = conn
	s.quixStream = quixStream
}
func (s *QStream) onEnd() {
	s.conn = nil
	s.quixStream = nil
}

func (s *QStream) Write(p []byte) (n int, err error) {
	// TODO
	return
}
func (s *QStream) Read(p []byte) (n int, err error) {
	// TODO
	return
}