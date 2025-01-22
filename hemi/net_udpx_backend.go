// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// UDPX (UDP/UDS) backend implementation. See RFC 768 and RFC 8085.

package hemi

import (
	"net"
	"sync"
	"syscall"
	"time"
)

func init() {
	RegisterBackend("udpxBackend", func(compName string, stage *Stage) Backend {
		b := new(UDPXBackend)
		b.onCreate(compName, stage)
		return b
	})
}

// UDPXBackend component.
type UDPXBackend struct {
	// Parent
	Backend_[*udpxNode]
	// States
}

func (b *UDPXBackend) onCreate(compName string, stage *Stage) {
	b.Backend_.OnCreate(compName, stage)
}

func (b *UDPXBackend) OnConfigure() {
	b.Backend_.OnConfigure()

	// sub components
	b.ConfigureNodes()
}
func (b *UDPXBackend) OnPrepare() {
	b.Backend_.OnPrepare()

	// sub components
	b.PrepareNodes()
}

func (b *UDPXBackend) CreateNode(compName string) Node {
	node := new(udpxNode)
	node.onCreate(compName, b.stage, b)
	b.AddNode(node)
	return node
}

func (b *UDPXBackend) Dial() (*UConn, error) {
	node := b.nodes[b.nodeIndexGet()]
	return node.dial()
}

// udpxNode is a node in UDPXBackend.
type udpxNode struct {
	// Parent
	Node_[*UDPXBackend]
	// Mixins
	_udpxHolder_
	// States
}

func (n *udpxNode) onCreate(compName string, stage *Stage, backend *UDPXBackend) {
	n.Node_.OnCreate(compName, stage, backend)
}

func (n *udpxNode) OnConfigure() {
	n.Node_.OnConfigure()
	n._udpxHolder_.onConfigure(n)
}
func (n *udpxNode) OnPrepare() {
	n.Node_.OnPrepare()
	n._udpxHolder_.onPrepare(n)
}

func (n *udpxNode) Maintain() { // runner
	n.LoopRun(time.Second, func(now time.Time) {
		// TODO: health check, markDown, markUp()
	})
	n.markDown()
	// TODO: wait for all conns
	if DebugLevel() >= 2 {
		Printf("udpxNode=%s done\n", n.compName)
	}
	n.backend.DecSub() // node
}

func (n *udpxNode) dial() (*UConn, error) {
	// TODO. note: use n.IncSubConns()?
	return nil, nil
}

// UConn
type UConn struct {
	// Parent
	udpxConn_
	// Conn states (non-zeros)
	node *udpxNode
	// Conn states (zeros)
}

var poolUConn sync.Pool

func getUConn(id int64, node *udpxNode, pktConn net.PacketConn, rawConn syscall.RawConn) *UConn {
	var conn *UConn
	if x := poolUConn.Get(); x == nil {
		conn = new(UConn)
	} else {
		conn = x.(*UConn)
	}
	conn.onGet(id, node, pktConn, rawConn)
	return conn
}
func putUConn(conn *UConn) {
	conn.onPut()
	poolUConn.Put(conn)
}

func (c *UConn) onGet(id int64, node *udpxNode, pktConn net.PacketConn, rawConn syscall.RawConn) {
	c.udpxConn_.onGet(id, node.Stage(), pktConn, rawConn, node.UDSMode())

	c.node = node
}
func (c *UConn) onPut() {
	c.node = nil

	c.udpxConn_.onPut()
}

func (c *UConn) Close() error {
	// TODO: c.node.DecSubConns()?
	pktConn := c.pktConn
	putUConn(c)
	return pktConn.Close()
}
