// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// QUIC client implementation.

package internal

import (
	"fmt"
	"github.com/hexinfra/gorox/hemi/libraries/quix"
	"sync"
	"sync/atomic"
	"time"
)

func init() {
	registerFixture(signQUIC)
	registerBackend("quicBackend", func(name string, stage *Stage) backend {
		b := new(QUICBackend)
		b.onCreate(name, stage)
		return b
	})
}

// quicClient is the interface for QUICOutgate and QUICBackend.
type quicClient interface {
	client
	streamHolder
}

// quicClient_
type quicClient_ struct {
	// Mixins
	streamHolder_
	// States
}

func (q *quicClient_) onCreate() {
}

func (q *quicClient_) onConfigure(shell Component) {
	q.streamHolder_.onConfigure(shell, 1000)
}
func (q *quicClient_) onPrepare(shell Component) {
	q.streamHolder_.onPrepare(shell)
}

const signQUIC = "quic"

func createQUIC(stage *Stage) *QUICOutgate {
	quic := new(QUICOutgate)
	quic.onCreate(stage)
	quic.setShell(quic)
	return quic
}

// QUICOutgate component.
type QUICOutgate struct {
	// Mixins
	client_
	quicClient_
	// States
}

func (f *QUICOutgate) onCreate(stage *Stage) {
	f.client_.onCreate(signQUIC, stage)
	f.quicClient_.onCreate()
}
func (f *QUICOutgate) OnShutdown() {
	f.Shutdown()
}

func (f *QUICOutgate) OnConfigure() {
	f.client_.onConfigure()
	f.quicClient_.onConfigure(f)
}
func (f *QUICOutgate) OnPrepare() {
	f.client_.onPrepare()
	f.quicClient_.onPrepare(f)
}

func (f *QUICOutgate) run() { // goroutine
	Loop(time.Second, f.Shut, func(now time.Time) {
		// TODO
	})
	if Debug(2) {
		fmt.Println("quic done")
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
func (f *QUICOutgate) StoreConn(conn *QConn) {
	// TODO
}

// QUICBackend component.
type QUICBackend struct {
	// Mixins
	backend_[*quicNode]
	quicClient_
	loadBalancer_
	// States
	health any // TODO
}

func (b *QUICBackend) onCreate(name string, stage *Stage) {
	b.backend_.onCreate(name, stage, b)
	b.quicClient_.onCreate()
	b.loadBalancer_.init()
}
func (b *QUICBackend) OnShutdown() {
	b.Shutdown()
}

func (b *QUICBackend) OnConfigure() {
	b.backend_.onConfigure()
	b.quicClient_.onConfigure(b)
	b.loadBalancer_.onConfigure(b)
}
func (b *QUICBackend) OnPrepare() {
	b.backend_.onPrepare()
	b.quicClient_.onPrepare(b)
	b.loadBalancer_.onPrepare(len(b.nodes))
}

func (b *QUICBackend) createNode(id int32) *quicNode {
	n := new(quicNode)
	n.init(id, b)
	return n
}

func (b *QUICBackend) Dial() (*QConn, error) {
	// TODO
	return nil, nil
}
func (b *QUICBackend) FetchConn() (*QConn, error) {
	// TODO
	return nil, nil
}
func (b *QUICBackend) StoreConn(conn *QConn) {
	// TODO
}

// quicNode is a node in QUICBackend.
type quicNode struct {
	// Mixins
	node_
	// Assocs
	backend *QUICBackend
	// States
}

func (n *quicNode) init(id int32, backend *QUICBackend) {
	n.node_.init(id)
	n.backend = backend
}

func (n *quicNode) maintain(shut chan struct{}) { // goroutine
	Loop(time.Second, shut, func(now time.Time) {
		// TODO: health check
	})
	// TODO: wait for all conns
	if Debug(2) {
		fmt.Printf("quicNode=%d done\n", n.id)
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
func (n *quicNode) storeConn(conn *QConn) {
	// Note: A QConn can be used concurrently, limited by maxStreams.
	// TODO
}

// poolQConn
var poolQConn sync.Pool

func getQConn(id int64, client quicClient, node *quicNode, quicConn *quix.Conn) *QConn {
	var conn *QConn
	if x := poolQConn.Get(); x == nil {
		conn = new(QConn)
	} else {
		conn = x.(*QConn)
	}
	conn.onGet(id, client, node, quicConn)
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
	node       *quicNode // associated node if client is QUICBackend
	quicConn   *quix.Conn
	maxStreams int32 // how many streams are allowed on this conn?
	// Conn states (zeros)
	usedStreams atomic.Int32 // how many streams has been used?
	broken      atomic.Bool  // is conn broken?
}

func (c *QConn) onGet(id int64, client quicClient, node *quicNode, quicConn *quix.Conn) {
	c.conn_.onGet(id, client)
	c.node = node
	c.quicConn = quicConn
	c.maxStreams = client.MaxStreamsPerConn()
}
func (c *QConn) onPut() {
	c.conn_.onPut()
	c.node = nil
	c.quicConn = nil
	c.usedStreams.Store(0)
	c.broken.Store(false)
}

func (c *QConn) getClient() quicClient { return c.client.(quicClient) }

func (c *QConn) isBroken() bool { return c.broken.Load() }
func (c *QConn) markBroken()    { c.broken.Store(true) }

func (c *QConn) reachLimit() bool {
	return c.usedStreams.Add(1) > c.maxStreams
}

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
