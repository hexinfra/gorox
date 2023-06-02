// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// QUDS (QUIC over UUDS) client implementation.

package internal

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/hexinfra/gorox/hemi/common/quix"
)

func init() {
	registerFixture(signQUDSOutgate)
	RegisterBackend("qudsBackend", func(name string, stage *Stage) Backend {
		b := new(QUDSBackend)
		b.onCreate(name, stage)
		return b
	})
}

const signQUDSOutgate = "qudsOutgate"

func createQUDSOutgate(stage *Stage) *QUDSOutgate {
	quds := new(QUDSOutgate)
	quds.onCreate(stage)
	quds.setShell(quds)
	return quds
}

// QUDSOutgate component.
type QUDSOutgate struct {
	// Mixins
	outgate_
	streamHolder_
	// States
}

func (f *QUDSOutgate) onCreate(stage *Stage) {
	f.outgate_.onCreate(signQUDSOutgate, stage)
}

func (f *QUDSOutgate) OnConfigure() {
	f.outgate_.onConfigure()
	f.streamHolder_.onConfigure(f, 1000)
}
func (f *QUDSOutgate) OnPrepare() {
	f.outgate_.onPrepare()
	f.streamHolder_.onPrepare(f)
}

func (f *QUDSOutgate) run() { // goroutine
	f.Loop(time.Second, func(now time.Time) {
		// TODO
	})
	if IsDebug(2) {
		Println("qudsOutgate done")
	}
	f.stage.SubDone()
}

func (f *QUDSOutgate) Dial(address string) (*DConn, error) {
	// TODO
	return nil, nil
}
func (f *QUDSOutgate) FetchConn(address string) {
	// TODO
}
func (f *QUDSOutgate) StoreConn(dConn *DConn) {
	// TODO
}

// QUDSBackend component.
type QUDSBackend struct {
	// Mixins
	Backend_[*qudsNode]
	streamHolder_
	loadBalancer_
	// States
	health any // TODO
}

func (b *QUDSBackend) onCreate(name string, stage *Stage) {
	b.Backend_.onCreate(name, stage, b)
	b.loadBalancer_.init()
}

func (b *QUDSBackend) OnConfigure() {
	b.Backend_.onConfigure()
	b.streamHolder_.onConfigure(b, 1000)
	b.loadBalancer_.onConfigure(b)
}
func (b *QUDSBackend) OnPrepare() {
	b.Backend_.onPrepare()
	b.streamHolder_.onPrepare(b)
	b.loadBalancer_.onPrepare(len(b.nodes))
}

func (b *QUDSBackend) createNode(id int32) *qudsNode {
	node := new(qudsNode)
	node.init(id, b)
	return node
}

func (b *QUDSBackend) Dial() (*DConn, error) {
	// TODO
	return nil, nil
}
func (b *QUDSBackend) FetchConn() (*DConn, error) {
	// TODO
	return nil, nil
}
func (b *QUDSBackend) StoreConn(dConn *DConn) {
	// TODO
}

// qudsNode is a node in QUDSBackend.
type qudsNode struct {
	// Mixins
	Node_
	// Assocs
	backend *QUDSBackend
	// States
}

func (n *qudsNode) init(id int32, backend *QUDSBackend) {
	n.Node_.init(id)
	n.backend = backend
}

func (n *qudsNode) Maintain() { // goroutine
	n.Loop(time.Second, func(now time.Time) {
		// TODO: health check
	})
	// TODO: wait for all conns
	if IsDebug(2) {
		Printf("qudsNode=%d done\n", n.id)
	}
	n.backend.SubDone()
}

func (n *qudsNode) dial() (*DConn, error) {
	// TODO
	return nil, nil
}
func (n *qudsNode) fetchConn() (*DConn, error) {
	// Note: A DConn can be used concurrently, limited by maxStreams.
	// TODO
	return nil, nil
}
func (n *qudsNode) storeConn(dConn *DConn) {
	// Note: A DConn can be used concurrently, limited by maxStreams.
	// TODO
}

// poolDConn
var poolDConn sync.Pool

func getDConn(id int64, client qClient, node *qudsNode, quicConn *quix.Conn) *DConn {
	var conn *DConn
	if x := poolDConn.Get(); x == nil {
		conn = new(DConn)
	} else {
		conn = x.(*DConn)
	}
	conn.onGet(id, client, node, quicConn)
	return conn
}
func putDConn(conn *DConn) {
	conn.onPut()
	poolDConn.Put(conn)
}

// DConn is a client-side connection to qudsNode.
type DConn struct {
	// Mixins
	Conn_
	// Conn states (non-zeros)
	node       *qudsNode // associated node if client is QUDSBackend
	quicConn   *quix.Conn
	maxStreams int32 // how many streams are allowed on this conn?
	// Conn states (zeros)
	usedStreams atomic.Int32 // how many streams has been used?
	broken      atomic.Bool  // is conn broken?
}

func (c *DConn) onGet(id int64, client qClient, node *qudsNode, quicConn *quix.Conn) {
	c.Conn_.onGet(id, client)
	c.node = node
	c.quicConn = quicConn
	c.maxStreams = client.MaxStreamsPerConn()
}
func (c *DConn) onPut() {
	c.Conn_.onPut()
	c.node = nil
	c.quicConn = nil
	c.usedStreams.Store(0)
	c.broken.Store(false)
}

func (c *DConn) getClient() qClient { return c.client.(qClient) }

func (c *DConn) reachLimit() bool {
	return c.usedStreams.Add(1) > c.maxStreams
}

func (c *DConn) isBroken() bool { return c.broken.Load() }
func (c *DConn) markBroken()    { c.broken.Store(true) }

func (c *DConn) FetchStream() *DStream {
	// TODO
	return nil
}
func (c *DConn) StoreStream(stream *DStream) {
	// TODO
}
func (c *DConn) FetchOneway() *DOneway {
	// TODO
	return nil
}
func (c *DConn) StoreOneway(oneway *DOneway) {
	// TODO
}

// poolDStream
var poolDStream sync.Pool

func getDStream(conn *DConn, quicStream *quix.Stream) *DStream {
	var stream *DStream
	if x := poolDStream.Get(); x == nil {
		stream = new(DStream)
	} else {
		stream = x.(*DStream)
	}
	stream.onUse(conn, quicStream)
	return stream
}
func putDStream(stream *DStream) {
	stream.onEnd()
	poolDStream.Put(stream)
}

// DStream is a bidirectional stream of DConn.
type DStream struct {
	// TODO
	conn       *DConn
	quicStream *quix.Stream
}

func (s *DStream) onUse(conn *DConn, quicStream *quix.Stream) {
	s.conn = conn
	s.quicStream = quicStream
}
func (s *DStream) onEnd() {
	s.conn = nil
	s.quicStream = nil
}

func (s *DStream) Write(p []byte) (n int, err error) {
	// TODO
	return
}
func (s *DStream) Read(p []byte) (n int, err error) {
	// TODO
	return
}

// poolDOneway
var poolDOneway sync.Pool

func getDOneway(conn *DConn, quicOneway *quix.Oneway) *DOneway {
	var oneway *DOneway
	if x := poolDOneway.Get(); x == nil {
		oneway = new(DOneway)
	} else {
		oneway = x.(*DOneway)
	}
	oneway.onUse(conn, quicOneway)
	return oneway
}
func putDOneway(oneway *DOneway) {
	oneway.onEnd()
	poolDOneway.Put(oneway)
}

// DOneway is a unidirectional stream of DConn.
type DOneway struct {
	// TODO
	conn       *DConn
	quicOneway *quix.Oneway
}

func (s *DOneway) onUse(conn *DConn, quicOneway *quix.Oneway) {
	s.conn = conn
	s.quicOneway = quicOneway
}
func (s *DOneway) onEnd() {
	s.conn = nil
	s.quicOneway = nil
}

func (s *DOneway) Write(p []byte) (n int, err error) {
	// TODO
	return
}
func (s *DOneway) Read(p []byte) (n int, err error) {
	// TODO
	return
}
