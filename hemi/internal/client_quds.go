// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
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
	if Debug() >= 2 {
		Println("qudsOutgate done")
	}
	f.stage.SubDone()
}

func (f *QUDSOutgate) Dial(address string) (*XConnection, error) {
	// TODO
	return nil, nil
}
func (f *QUDSOutgate) FetchConnection(address string) {
	// TODO
}
func (f *QUDSOutgate) StoreConnection(xConnection *XConnection) {
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

func (b *QUDSBackend) Dial() (*XConnection, error) {
	// TODO
	return nil, nil
}
func (b *QUDSBackend) FetchConnection() (*XConnection, error) {
	// TODO
	return nil, nil
}
func (b *QUDSBackend) StoreConnection(xConnection *XConnection) {
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

func (n *qudsNode) setAddress(address string) {
	n.Node_.setAddress(address)
	// TODO
}

func (n *qudsNode) Maintain() { // goroutine
	n.Loop(time.Second, func(now time.Time) {
		// TODO: health check
	})
	// TODO: wait for all conns
	if Debug() >= 2 {
		Printf("qudsNode=%d done\n", n.id)
	}
	n.backend.SubDone()
}

func (n *qudsNode) dial() (*XConnection, error) {
	// TODO
	return nil, nil
}
func (n *qudsNode) fetchConnection() (*XConnection, error) {
	// Note: An XConnection can be used concurrently, limited by maxStreams.
	// TODO
	return nil, nil
}
func (n *qudsNode) storeConnection(xConnection *XConnection) {
	// Note: An XConnection can be used concurrently, limited by maxStreams.
	// TODO
}

// poolXConnection
var poolXConnection sync.Pool

func getXConnection(id int64, client qClient, node *qudsNode, quicConnection *quix.Connection) *XConnection {
	var connection *XConnection
	if x := poolXConnection.Get(); x == nil {
		connection = new(XConnection)
	} else {
		connection = x.(*XConnection)
	}
	connection.onGet(id, client, node, quicConnection)
	return connection
}
func putXConnection(connection *XConnection) {
	connection.onPut()
	poolXConnection.Put(connection)
}

// XConnection is a client-side connection to qudsNode.
type XConnection struct {
	// Mixins
	Conn_
	// Connection states (non-zeros)
	node           *qudsNode // associated node if client is QUDSBackend
	quicConnection *quix.Connection
	maxStreams     int32 // how many streams are allowed on this connection?
	// Connection states (zeros)
	usedStreams atomic.Int32 // how many streams has been used?
	broken      atomic.Bool  // is connection broken?
}

func (c *XConnection) onGet(id int64, client qClient, node *qudsNode, quicConnection *quix.Connection) {
	c.Conn_.onGet(id, client)
	c.node = node
	c.quicConnection = quicConnection
	c.maxStreams = client.MaxStreamsPerConn()
}
func (c *XConnection) onPut() {
	c.Conn_.onPut()
	c.node = nil
	c.quicConnection = nil
	c.usedStreams.Store(0)
	c.broken.Store(false)
}

func (c *XConnection) getClient() qClient { return c.client.(qClient) }

func (c *XConnection) reachLimit() bool {
	return c.usedStreams.Add(1) > c.maxStreams
}

func (c *XConnection) isBroken() bool { return c.broken.Load() }
func (c *XConnection) markBroken()    { c.broken.Store(true) }

func (c *XConnection) FetchStream() *XStream {
	// TODO
	return nil
}
func (c *XConnection) StoreStream(stream *XStream) {
	// TODO
}
func (c *XConnection) FetchOneway() *XOneway {
	// TODO
	return nil
}
func (c *XConnection) StoreOneway(oneway *XOneway) {
	// TODO
}

// poolXStream
var poolXStream sync.Pool

func getXStream(connection *XConnection, quicStream *quix.Stream) *XStream {
	var stream *XStream
	if x := poolXStream.Get(); x == nil {
		stream = new(XStream)
	} else {
		stream = x.(*XStream)
	}
	stream.onUse(connection, quicStream)
	return stream
}
func putXStream(stream *XStream) {
	stream.onEnd()
	poolXStream.Put(stream)
}

// XStream is a bidirectional stream of XConnection.
type XStream struct {
	// TODO
	connection *XConnection
	quicStream *quix.Stream
}

func (s *XStream) onUse(connection *XConnection, quicStream *quix.Stream) {
	s.connection = connection
	s.quicStream = quicStream
}
func (s *XStream) onEnd() {
	s.connection = nil
	s.quicStream = nil
}

func (s *XStream) Write(p []byte) (n int, err error) {
	// TODO
	return
}
func (s *XStream) Read(p []byte) (n int, err error) {
	// TODO
	return
}

// poolXOneway
var poolXOneway sync.Pool

func getXOneway(connection *XConnection, quicOneway *quix.Oneway) *XOneway {
	var oneway *XOneway
	if x := poolXOneway.Get(); x == nil {
		oneway = new(XOneway)
	} else {
		oneway = x.(*XOneway)
	}
	oneway.onUse(connection, quicOneway)
	return oneway
}
func putXOneway(oneway *XOneway) {
	oneway.onEnd()
	poolXOneway.Put(oneway)
}

// XOneway is a unidirectional stream of XConnection.
type XOneway struct {
	// TODO
	connection *XConnection
	quicOneway *quix.Oneway
}

func (s *XOneway) onUse(connection *XConnection, quicOneway *quix.Oneway) {
	s.connection = connection
	s.quicOneway = quicOneway
}
func (s *XOneway) onEnd() {
	s.connection = nil
	s.quicOneway = nil
}

func (s *XOneway) Write(p []byte) (n int, err error) {
	// TODO
	return
}
func (s *XOneway) Read(p []byte) (n int, err error) {
	// TODO
	return
}
