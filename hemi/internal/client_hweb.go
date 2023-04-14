// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HWEB client implementation.

package internal

import (
	"net"
	"sync"
	"syscall"
	"time"
)

// hwebBackend
type hwebBackend struct {
	// Mixins
	webBackend_[*hwebNode]
}

func (b *hwebBackend) onCreate(name string, stage *Stage) {
	b.webBackend_.onCreate(name, stage, b)
}

func (b *hwebBackend) OnConfigure() {
	b.webBackend_.onConfigure(b)
}
func (b *hwebBackend) OnPrepare() {
	b.webBackend_.onPrepare(b, len(b.nodes))
}

func (b *hwebBackend) createNode(id int32) *hwebNode {
	node := new(hwebNode)
	node.init(id, b)
	return node
}

func (b *hwebBackend) FetchConn() (*hConn, error) {
	node := b.nodes[b.getNext()]
	return node.fetchConn()
}
func (b *hwebBackend) StoreConn(conn *hConn) {
	conn.node.storeConn(conn)
}

// hwebNode
type hwebNode struct {
	// Mixins
	webNode_
	// Assocs
	backend *hwebBackend
	// States
}

func (n *hwebNode) init(id int32, backend *hwebBackend) {
	n.webNode_.init(id)
	n.backend = backend
}

func (n *hwebNode) maintain(shut chan struct{}) { // goroutine
	Loop(time.Second, shut, func(now time.Time) {
		// TODO: health check
	})
	// TODO: wait for all conns
	if IsDebug(2) {
		Debugf("http2Node=%d done\n", n.id)
	}
	n.backend.SubDone()
}

func (n *hwebNode) fetchConn() (*hConn, error) {
	// Note: An hConn can be used concurrently, limited by maxStreams.
	// TODO
	return nil, nil
}
func (n *hwebNode) storeConn(wConn *hConn) {
	// Note: An hConn can be used concurrently, limited by maxStreams.
	// TODO
}

// hConn
type hConn struct {
	// Mixins
	wConn_
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	node    *hwebNode // associated node
	netConn net.Conn  // the connection (TCP/TLS)
	rawConn syscall.RawConn
	// Conn states (zeros)
	activeStreams int32 // concurrent streams
}

func (c *hConn) onGet(id int64, client webClient, node *hwebNode, netConn net.Conn, rawConn syscall.RawConn) {
	c.wConn_.onGet(id, client)
	c.node = node
	c.netConn = netConn
	c.rawConn = rawConn
}
func (c *hConn) onPut() {
	c.wConn_.onPut()
	c.node = nil
	c.netConn = nil
	c.rawConn = nil
	c.activeStreams = 0
}

// poolHStream
var poolHStream sync.Pool

func getHStream(conn *hConn, id uint32) *hStream {
	return nil
}
func putHStream(stream *hStream) {
}

// hStream
type hStream struct {
	// Mixins
	wStream_
	// Assocs
	request  hRequest
	response hResponse
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	conn *hConn
	id   uint32
	// Stream states (zeros)
	hStream0 // all values must be zero by default in this struct!
}
type hStream0 struct { // for fast reset, entirely
}

// hRequest is the client-side HWEB request.
type hRequest struct {
	// Mixins
	wRequest_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}

// hResponse is the client-side HWEB response.
type hResponse struct {
	// Mixins
	wResponse_
	// Stream states (stocks)
	// Stream states (controlled)
	// Stream states (non-zeros)
	// Stream states (zeros)
}
