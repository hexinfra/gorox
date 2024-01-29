// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// QUIC client implementation.

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

func (f *QUICOutgate) run() { // runner
	f.Loop(time.Second, func(now time.Time) {
		// TODO
	})
	if Debug() >= 2 {
		Println("quicOutgate done")
	}
	f.stage.SubDone()
}

func (f *QUICOutgate) Dial(address string) (*QConnection, error) {
	// TODO
	return nil, nil
}
func (f *QUICOutgate) FetchConnection(address string) {
	// TODO
}
func (f *QUICOutgate) StoreConnection(qConnection *QConnection) {
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

func (b *QUICBackend) Dial() (*QConnection, error) {
	// TODO
	return nil, nil
}
func (b *QUICBackend) FetchConnection() (*QConnection, error) {
	// TODO
	return nil, nil
}
func (b *QUICBackend) StoreConnection(qConnection *QConnection) {
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

func (n *quicNode) dial() (*QConnection, error) {
	// TODO
	return nil, nil
}
func (n *quicNode) fetchConnection() (*QConnection, error) {
	// Note: A QConnection can be used concurrently, limited by maxStreams.
	// TODO
	return nil, nil
}
func (n *quicNode) storeConnection(qConnection *QConnection) {
	// Note: A QConnection can be used concurrently, limited by maxStreams.
	// TODO
}

// poolQConnection
var poolQConnection sync.Pool

func getQConnection(id int64, client qClient, node *quicNode, quicConnection *quix.Connection) *QConnection {
	var connection *QConnection
	if x := poolQConnection.Get(); x == nil {
		connection = new(QConnection)
	} else {
		connection = x.(*QConnection)
	}
	connection.onGet(id, client, node, quicConnection)
	return connection
}
func putQConnection(connection *QConnection) {
	connection.onPut()
	poolQConnection.Put(connection)
}

// QConnection is a client-side connection to quicNode.
type QConnection struct {
	// Mixins
	Conn_
	// Connection states (non-zeros)
	node           *quicNode // associated node if client is QUICBackend
	quicConnection *quix.Connection
	maxStreams     int32 // how many streams are allowed on this connection?
	// Connection states (zeros)
	usedStreams atomic.Int32 // how many streams has been used?
	broken      atomic.Bool  // is connection broken?
}

func (c *QConnection) onGet(id int64, client qClient, node *quicNode, quicConnection *quix.Connection) {
	c.Conn_.onGet(id, client)
	c.node = node
	c.quicConnection = quicConnection
	c.maxStreams = client.MaxStreamsPerConn()
}
func (c *QConnection) onPut() {
	c.Conn_.onPut()
	c.node = nil
	c.quicConnection = nil
	c.usedStreams.Store(0)
	c.broken.Store(false)
}

func (c *QConnection) getClient() qClient { return c.client.(qClient) }

func (c *QConnection) reachLimit() bool {
	return c.usedStreams.Add(1) > c.maxStreams
}

func (c *QConnection) isBroken() bool { return c.broken.Load() }
func (c *QConnection) markBroken()    { c.broken.Store(true) }

func (c *QConnection) FetchStream() *QStream {
	// TODO
	return nil
}
func (c *QConnection) StoreStream(stream *QStream) {
	// TODO
}
func (c *QConnection) FetchOneway() *QOneway {
	// TODO
	return nil
}
func (c *QConnection) StoreOneway(oneway *QOneway) {
	// TODO
}

// poolQStream
var poolQStream sync.Pool

func getQStream(connection *QConnection, quicStream *quix.Stream) *QStream {
	var stream *QStream
	if x := poolQStream.Get(); x == nil {
		stream = new(QStream)
	} else {
		stream = x.(*QStream)
	}
	stream.onUse(connection, quicStream)
	return stream
}
func putQStream(stream *QStream) {
	stream.onEnd()
	poolQStream.Put(stream)
}

// QStream is a bidirectional stream of QConnection.
type QStream struct {
	// TODO
	connection *QConnection
	quicStream *quix.Stream
}

func (s *QStream) onUse(connection *QConnection, quicStream *quix.Stream) {
	s.connection = connection
	s.quicStream = quicStream
}
func (s *QStream) onEnd() {
	s.connection = nil
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

func getQOneway(connection *QConnection, quicOneway *quix.Oneway) *QOneway {
	var oneway *QOneway
	if x := poolQOneway.Get(); x == nil {
		oneway = new(QOneway)
	} else {
		oneway = x.(*QOneway)
	}
	oneway.onUse(connection, quicOneway)
	return oneway
}
func putQOneway(oneway *QOneway) {
	oneway.onEnd()
	poolQOneway.Put(oneway)
}

// QOneway is a unidirectional stream of QConnection.
type QOneway struct {
	// TODO
	connection *QConnection
	quicOneway *quix.Oneway
}

func (s *QOneway) onUse(connection *QConnection, quicOneway *quix.Oneway) {
	s.connection = connection
	s.quicOneway = quicOneway
}
func (s *QOneway) onEnd() {
	s.connection = nil
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
