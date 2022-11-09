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

// quicClient is the interface for QUICOutgate and QUICBackend.
type quicClient interface {
	client
	streamHolder
}

func init() {
	registerFixture(signQUIC)
	registerBackend("quicBackend", func(name string, stage *Stage) backend {
		b := new(QUICBackend)
		b.init(name, stage)
		return b
	})
}

const signQUIC = "quic"

func createQUIC(stage *Stage) *QUICOutgate {
	quic := new(QUICOutgate)
	quic.init(stage)
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

func (f *QUICOutgate) init(stage *Stage) {
	f.outgate_.init(signQUIC, stage)
}

func (f *QUICOutgate) OnConfigure() {
	f.outgate_.onConfigure()
	// maxStreamsPerConn
	f.ConfigureInt32("maxStreamsPerConn", &f.maxStreamsPerConn, func(value int32) bool { return value > 0 }, 1000)
}
func (f *QUICOutgate) OnPrepare() {
	f.outgate_.onPrepare()
}
func (f *QUICOutgate) OnShutdown() {
	f.outgate_.onShutdown()
}

func (f *QUICOutgate) run() { // goroutine
	for !f.IsShut() {
		// TODO
		time.Sleep(time.Second)
	}
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
	backend_
	streamHolder_
	// States
	healthCheck any         // TODO
	nodes       []*quicNode // nodes of backend
}

func (b *QUICBackend) init(name string, stage *Stage) {
	b.backend_.init(name, stage)
}

func (b *QUICBackend) OnConfigure() {
	b.backend_.onConfigure()
	// maxStreamsPerConn
	b.ConfigureInt32("maxStreamsPerConn", &b.maxStreamsPerConn, func(value int32) bool { return value > 0 }, 1000)
}
func (b *QUICBackend) OnPrepare() {
	b.backend_.onPrepare(len(b.nodes))
}
func (b *QUICBackend) OnShutdown() {
	b.backend_.onShutdown()
}

func (b *QUICBackend) maintain() { // goroutine
	// TODO: health check for all nodes
	for {
		time.Sleep(time.Second)
	}
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

func (n *quicNode) checkHealth() {
	// TODO
	for {
		time.Sleep(time.Second)
	}
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

// QOneway is a unidirectional stream of QConn.
type QOneway struct {
	// TODO
	oneway *quix.Oneway
}

func (s *QOneway) Write(p []byte) (n int, err error) {
	// TODO
	return
}
func (s *QOneway) Read(p []byte) (n int, err error) {
	// TODO
	return
}
