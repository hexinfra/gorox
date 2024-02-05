// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// UDPS (UDP/TLS/UDS) network client implementation.

package internal

import (
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

func init() {
	RegisterBackend("udpsBackend", func(name string, stage *Stage) Backend {
		b := new(UDPSBackend)
		b.onCreate(name, stage)
		return b
	})
}

// UDPSBackend component.
type UDPSBackend struct {
	// Mixins
	Backend_[*udpsNode]
	loadBalancer_
	// States
	health any // TODO
}

func (b *UDPSBackend) onCreate(name string, stage *Stage) {
	b.Backend_.onCreate(name, stage, b)
	b.loadBalancer_.init()
}

func (b *UDPSBackend) OnConfigure() {
	b.Backend_.onConfigure()
	b.loadBalancer_.onConfigure(b)
}
func (b *UDPSBackend) OnPrepare() {
	b.Backend_.onPrepare()
	b.loadBalancer_.onPrepare(len(b.nodes))
}

func (b *UDPSBackend) createNode(id int32) *udpsNode {
	node := new(udpsNode)
	node.init(id, b)
	return node
}

func (b *UDPSBackend) Conn() (*UConn, error) {
	node := b.nodes[b.getNext()]
	return node.conn()
}
func (b *UDPSBackend) FetchConn() (*UConn, error) {
	node := b.nodes[b.getNext()]
	return node.fetchConn()
}
func (b *UDPSBackend) StoreConn(uConn *UConn) {
	uConn.node.storeConn(uConn)
}

// udpsNode is a node in UDPSBackend.
type udpsNode struct {
	// Mixins
	Node_
	// Assocs
	backend *UDPSBackend
	// States
}

func (n *udpsNode) init(id int32, backend *UDPSBackend) {
	n.Node_.init(id)
	n.backend = backend
}

func (n *udpsNode) Maintain() { // runner
	n.Loop(time.Second, func(now time.Time) {
		// TODO: health check
	})
	// TODO: wait for all conns
	if Debug() >= 2 {
		Printf("udpsNode=%d done\n", n.id)
	}
	n.backend.SubDone()
}

func (n *udpsNode) conn() (*UConn, error) {
	// TODO
	return nil, nil
}

func (n *udpsNode) fetchConn() (*UConn, error) {
	conn := n.pullConn()
	if conn != nil {
		uConn := conn.(*UConn)
		if uConn.isAlive() {
			return uConn, nil
		}
		uConn.closeConn()
		putUConn(uConn)
	}
	return n.conn()
}
func (n *udpsNode) storeConn(uConn *UConn) {
	if uConn.isBroken() || n.isDown() || !uConn.isAlive() {
		uConn.closeConn()
		putUConn(uConn)
	} else {
		n.pushConn(uConn)
	}
}

// poolUConn
var poolUConn sync.Pool

func getUConn(id int64, udsMode bool, tlsMode bool, backend *UDPSBackend, node *udpsNode, netConn net.PacketConn, rawConn syscall.RawConn) *UConn {
	var conn *UConn
	if x := poolUConn.Get(); x == nil {
		conn = new(UConn)
	} else {
		conn = x.(*UConn)
	}
	conn.onGet(id, udsMode, tlsMode, backend, node, netConn, rawConn)
	return conn
}
func putUConn(conn *UConn) {
	conn.onPut()
	poolUConn.Put(conn)
}

// UConn
type UConn struct {
	// Mixins
	Conn_
	// Conn states (non-zeros)
	backend *UDPSBackend
	node    *udpsNode
	netConn net.PacketConn
	rawConn syscall.RawConn // for syscall
	// Conn states (zeros)
	broken atomic.Bool // is conn broken?
}

func (c *UConn) onGet(id int64, udsMode bool, tlsMode bool, backend *UDPSBackend, node *udpsNode, netConn net.PacketConn, rawConn syscall.RawConn) {
	c.Conn_.onGet(id, udsMode, tlsMode, time.Now().Add(backend.AliveTimeout()))
	c.backend = backend
	c.node = node
	c.netConn = netConn
	c.rawConn = rawConn
}
func (c *UConn) onPut() {
	c.Conn_.onPut()
	c.backend = nil
	c.node = nil
	c.netConn = nil
	c.rawConn = nil
	c.broken.Store(false)
}

func (c *UConn) Backend() *UDPSBackend { return c.backend }

func (c *UConn) SetWriteDeadline(deadline time.Time) error {
	if deadline.Sub(c.lastWrite) >= time.Second {
		if err := c.netConn.SetWriteDeadline(deadline); err != nil {
			return err
		}
		c.lastWrite = deadline
	}
	return nil
}
func (c *UConn) SetReadDeadline(deadline time.Time) error {
	if deadline.Sub(c.lastRead) >= time.Second {
		if err := c.netConn.SetReadDeadline(deadline); err != nil {
			return err
		}
		c.lastRead = deadline
	}
	return nil
}

func (c *UConn) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	return c.netConn.WriteTo(p, addr)
}
func (c *UConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) { return c.netConn.ReadFrom(p) }

func (c *UConn) isBroken() bool { return c.broken.Load() }
func (c *UConn) markBroken()    { c.broken.Store(true) }

func (c *UConn) Close() error { // only used by clients of dial
	netConn := c.netConn
	putUConn(c)
	return netConn.Close()
}

func (c *UConn) closeConn() { c.netConn.Close() } // used by codes which use fetch/store
