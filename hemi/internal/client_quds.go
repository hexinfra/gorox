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

func (f *QUDSOutgate) Dial(address string) (*UConn, error) {
	// TODO
	return nil, nil
}
func (f *QUDSOutgate) FetchConn(address string) {
	// TODO
}
func (f *QUDSOutgate) StoreConn(uConn *UConn) {
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

func (b *QUDSBackend) Dial() (*UConn, error) {
	// TODO
	return nil, nil
}
func (b *QUDSBackend) FetchConn() (*UConn, error) {
	// TODO
	return nil, nil
}
func (b *QUDSBackend) StoreConn(uConn *UConn) {
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

func (n *qudsNode) dial() (*UConn, error) {
	// TODO
	return nil, nil
}
func (n *qudsNode) fetchConn() (*UConn, error) {
	// Note: A UConn can be used concurrently, limited by maxStreams.
	// TODO
	return nil, nil
}
func (n *qudsNode) storeConn(uConn *UConn) {
	// Note: A UConn can be used concurrently, limited by maxStreams.
	// TODO
}

// poolUConn
var poolUConn sync.Pool

func getUConn(id int64, client qClient, node *qudsNode, quicConn *quix.Conn) *UConn {
	var conn *UConn
	if x := poolUConn.Get(); x == nil {
		conn = new(UConn)
	} else {
		conn = x.(*UConn)
	}
	conn.onGet(id, client, node, quicConn)
	return conn
}
func putUConn(conn *UConn) {
	conn.onPut()
	poolUConn.Put(conn)
}

// UConn is a client-side connection to qudsNode.
type UConn struct {
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

func (c *UConn) onGet(id int64, client qClient, node *qudsNode, quicConn *quix.Conn) {
	c.Conn_.onGet(id, client)
	c.node = node
	c.quicConn = quicConn
	c.maxStreams = client.MaxStreamsPerConn()
}
func (c *UConn) onPut() {
	c.Conn_.onPut()
	c.node = nil
	c.quicConn = nil
	c.usedStreams.Store(0)
	c.broken.Store(false)
}

func (c *UConn) getClient() qClient { return c.client.(qClient) }

func (c *UConn) reachLimit() bool {
	return c.usedStreams.Add(1) > c.maxStreams
}

func (c *UConn) isBroken() bool { return c.broken.Load() }
func (c *UConn) markBroken()    { c.broken.Store(true) }

func (c *UConn) FetchStream() *UStream {
	// TODO
	return nil
}
func (c *UConn) StoreStream(stream *UStream) {
	// TODO
}
func (c *UConn) FetchOneway() *UOneway {
	// TODO
	return nil
}
func (c *UConn) StoreOneway(oneway *UOneway) {
	// TODO
}

// poolUStream
var poolUStream sync.Pool

func getUStream(conn *UConn, quicStream *quix.Stream) *UStream {
	var stream *UStream
	if x := poolUStream.Get(); x == nil {
		stream = new(UStream)
	} else {
		stream = x.(*UStream)
	}
	stream.onUse(conn, quicStream)
	return stream
}
func putUStream(stream *UStream) {
	stream.onEnd()
	poolUStream.Put(stream)
}

// UStream is a bidirectional stream of UConn.
type UStream struct {
	// TODO
	conn       *UConn
	quicStream *quix.Stream
}

func (s *UStream) onUse(conn *UConn, quicStream *quix.Stream) {
	s.conn = conn
	s.quicStream = quicStream
}
func (s *UStream) onEnd() {
	s.conn = nil
	s.quicStream = nil
}

func (s *UStream) Write(p []byte) (n int, err error) {
	// TODO
	return
}
func (s *UStream) Read(p []byte) (n int, err error) {
	// TODO
	return
}

// poolUOneway
var poolUOneway sync.Pool

func getUOneway(conn *UConn, quicOneway *quix.Oneway) *UOneway {
	var oneway *UOneway
	if x := poolUOneway.Get(); x == nil {
		oneway = new(UOneway)
	} else {
		oneway = x.(*UOneway)
	}
	oneway.onUse(conn, quicOneway)
	return oneway
}
func putUOneway(oneway *UOneway) {
	oneway.onEnd()
	poolUOneway.Put(oneway)
}

// UOneway is a unidirectional stream of UConn.
type UOneway struct {
	// TODO
	conn       *UConn
	quicOneway *quix.Oneway
}

func (s *UOneway) onUse(conn *UConn, quicOneway *quix.Oneway) {
	s.conn = conn
	s.quicOneway = quicOneway
}
func (s *UOneway) onEnd() {
	s.conn = nil
	s.quicOneway = nil
}

func (s *UOneway) Write(p []byte) (n int, err error) {
	// TODO
	return
}
func (s *UOneway) Read(p []byte) (n int, err error) {
	// TODO
	return
}
