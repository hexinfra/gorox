// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// UDPS (UDP/TLS/UDS) backend implementation.

package hemi

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
	b.Backend_.OnCreate(name, stage, b.NewNode)
	b.loadBalancer_.init()
}

func (b *UDPSBackend) OnConfigure() {
	b.Backend_.OnConfigure()
	b.loadBalancer_.onConfigure(b)
}
func (b *UDPSBackend) OnPrepare() {
	b.Backend_.OnPrepare()
	b.loadBalancer_.onPrepare(len(b.nodes))
}

func (b *UDPSBackend) NewNode(id int32) *udpsNode {
	node := new(udpsNode)
	node.init(id, b)
	return node
}

func (b *UDPSBackend) Conn() (*UConn, error) {
	return b.nodes[b.getNext()].dial()
}
func (b *UDPSBackend) FetchConn() (*UConn, error) {
	return b.nodes[b.getNext()].fetchConn()
}
func (b *UDPSBackend) StoreConn(uConn *UConn) {
	uConn.node.(*udpsNode).storeConn(uConn)
}

// udpsNode is a node in UDPSBackend.
type udpsNode struct {
	// Mixins
	Node_
	// Assocs
	// States
}

func (n *udpsNode) init(id int32, backend *UDPSBackend) {
	n.Node_.Init(id, backend)
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

func (n *udpsNode) dial() (*UConn, error) {
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
		n.closeConn(uConn)
	}
	return n.dial()
}
func (n *udpsNode) storeConn(uConn *UConn) {
	if uConn.isBroken() || n.isDown() || !uConn.isAlive() {
		n.closeConn(uConn)
	} else {
		n.pushConn(uConn)
	}
}

func (n *udpsNode) closeConn(uConn *UConn) {
	uConn.Close()
	n.SubDone()
}

// poolUConn
var poolUConn sync.Pool

func getUConn(id int64, node *udpsNode, netConn net.PacketConn, rawConn syscall.RawConn) *UConn {
	var uConn *UConn
	if x := poolUConn.Get(); x == nil {
		uConn = new(UConn)
	} else {
		uConn = x.(*UConn)
	}
	uConn.onGet(id, node, netConn, rawConn)
	return uConn
}
func putUConn(uConn *UConn) {
	uConn.onPut()
	poolUConn.Put(uConn)
}

// UConn
type UConn struct {
	// Mixins
	BackendConn_
	// Conn states (non-zeros)
	netConn net.PacketConn
	rawConn syscall.RawConn // for syscall
	// Conn states (zeros)
	broken atomic.Bool // is conn broken?
}

func (c *UConn) onGet(id int64, node *udpsNode, netConn net.PacketConn, rawConn syscall.RawConn) {
	c.BackendConn_.OnGet(id, node)
	c.netConn = netConn
	c.rawConn = rawConn
}
func (c *UConn) onPut() {
	c.netConn = nil
	c.rawConn = nil
	c.broken.Store(false)
	c.BackendConn_.OnPut()
}

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

func (c *UConn) Close() error {
	netConn := c.netConn
	putUConn(c)
	return netConn.Close()
}
