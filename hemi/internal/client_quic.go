// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// QUIC client implementation.

package internal

import (
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
	registerBackend("quicBackend", func(name string, stage *Stage) backend {
		b := new(QUICBackend)
		b.init(name, stage)
		return b
	})
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

func (b *QUICBackend) maintain() { // blocking
	// TODO: health check for all nodes
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
	node       *quicNode // belonging node if client is QUICBackend
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

// QStream is a bidirectional stream of QConn.
type QStream struct {
	// TODO
	stream *quix.Stream
}

func (s *QStream) Write(p []byte) (n int, err error) {
	// TODO
	return
}
func (s *QStream) Read(p []byte) (n int, err error) {
	// TODO
	return
}

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
