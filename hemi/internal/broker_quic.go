// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// QUIC broker implementation.

package internal

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/hexinfra/gorox/hemi/common/quix"
)

func init() {
	registerFixture(signQUICOutgate)
	RegisterBackend("quicBackend", func(name string, stage *Stage) Backend {
		b := new(QUICBackend)
		b.onCreate(name, stage)
		return b
	})
}

// quicBroker is the interface for QUICOutgate and QUICBackend.
type quicBroker interface {
	client
	streamHolder
}

const signQUICOutgate = "quicOutgate"

func createQUICOutgate(stage *Stage) *QUICOutgate {
	quic := new(QUICOutgate)
	quic.onCreate(stage)
	quic.setShell(quic)
	return quic
}

// QUICOutgate component.
type QUICOutgate struct {
	// Mixins
	outgate_
	streamHolder_
	// States
}

func (f *QUICOutgate) onCreate(stage *Stage) {
	f.outgate_.onCreate(signQUICOutgate, stage)
}

func (f *QUICOutgate) OnConfigure() {
	f.outgate_.onConfigure()
	f.streamHolder_.onConfigure(f, 1000)
}
func (f *QUICOutgate) OnPrepare() {
	f.outgate_.onPrepare()
	f.streamHolder_.onPrepare(f)
}

func (f *QUICOutgate) run() { // goroutine
	f.Loop(time.Second, func(now time.Time) {
		// TODO
	})
	if IsDebug(2) {
		Println("quicOutgate done")
	}
	f.stage.SubDone()
}

func (f *QUICOutgate) Dial(address string) (*QConn, error) {
	// TODO
	return nil, nil
}
func (f *QUICOutgate) FetchConn(address string) {
	// TODO
}
func (f *QUICOutgate) StoreConn(qConn *QConn) {
	// TODO
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

func (n *quicNode) Maintain() { // goroutine
	n.Loop(time.Second, func(now time.Time) {
		// TODO: health check
	})
	// TODO: wait for all conns
	if IsDebug(2) {
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

func getQConn(id int64, broker quicBroker, node *quicNode, quicConn *quix.Conn) *QConn {
	var conn *QConn
	if x := poolQConn.Get(); x == nil {
		conn = new(QConn)
	} else {
		conn = x.(*QConn)
	}
	conn.onGet(id, broker, node, quicConn)
	return conn
}
func putQConn(conn *QConn) {
	conn.onPut()
	poolQConn.Put(conn)
}

// QConn is a client-side connection to quicNode.
type QConn struct {
	// Mixins
	conn_
	// Conn states (non-zeros)
	node       *quicNode // associated node if broker is QUICBackend
	quicConn   *quix.Conn
	maxStreams int32 // how many streams are allowed on this conn?
	// Conn states (zeros)
	usedStreams atomic.Int32 // how many streams has been used?
	broken      atomic.Bool  // is conn broken?
}

func (c *QConn) onGet(id int64, broker quicBroker, node *quicNode, quicConn *quix.Conn) {
	c.conn_.onGet(id, broker)
	c.node = node
	c.quicConn = quicConn
	c.maxStreams = broker.MaxStreamsPerConn()
}
func (c *QConn) onPut() {
	c.conn_.onPut()
	c.node = nil
	c.quicConn = nil
	c.usedStreams.Store(0)
	c.broken.Store(false)
}

func (c *QConn) getBroker() quicBroker { return c.client.(quicBroker) }

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
func (c *QConn) FetchOneway() *QOneway {
	// TODO
	return nil
}
func (c *QConn) StoreOneway(oneway *QOneway) {
	// TODO
}

// poolQStream
var poolQStream sync.Pool

func getQStream(conn *QConn, quicStream *quix.Stream) *QStream {
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
	quicStream *quix.Stream
}

func (s *QStream) onUse(conn *QConn, quicStream *quix.Stream) {
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

// poolQOneway
var poolQOneway sync.Pool

func getQOneway(conn *QConn, quicOneway *quix.Oneway) *QOneway {
	var oneway *QOneway
	if x := poolQOneway.Get(); x == nil {
		oneway = new(QOneway)
	} else {
		oneway = x.(*QOneway)
	}
	oneway.onUse(conn, quicOneway)
	return oneway
}
func putQOneway(oneway *QOneway) {
	oneway.onEnd()
	poolQOneway.Put(oneway)
}

// QOneway is a unidirectional stream of QConn.
type QOneway struct {
	// TODO
	conn       *QConn
	quicOneway *quix.Oneway
}

func (s *QOneway) onUse(conn *QConn, quicOneway *quix.Oneway) {
	s.conn = conn
	s.quicOneway = quicOneway
}
func (s *QOneway) onEnd() {
	s.conn = nil
	s.quicOneway = nil
}

func (s *QOneway) Write(p []byte) (n int, err error) {
	// TODO
	return
}
func (s *QOneway) Read(p []byte) (n int, err error) {
	// TODO
	return
}
