// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// UDPX (UDP/TLS/UDS) backend.

package hemi

import (
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

func init() {
	RegisterBackend("udpxBackend", func(name string, stage *Stage) Backend {
		b := new(UDPXBackend)
		b.onCreate(name, stage)
		return b
	})
}

// UDPXBackend component.
type UDPXBackend struct {
	// Parent
	Backend_[*udpxNode]
	// States
}

func (b *UDPXBackend) onCreate(name string, stage *Stage) {
	b.Backend_.OnCreate(name, stage)
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

func (b *UDPXBackend) CreateNode(name string) Node {
	node := new(udpxNode)
	node.onCreate(name, b)
	b.AddNode(node)
	return node
}

func (b *UDPXBackend) Dial() (*UConn, error) {
	node := b.nodes[b.nextIndex()]
	return node.dial()
}

// udpxNode is a node in UDPXBackend.
type udpxNode struct {
	// Parent
	Node_
	// Assocs
	backend *UDPXBackend
	// States
}

func (n *udpxNode) onCreate(name string, backend *UDPXBackend) {
	n.Node_.OnCreate(name)
	n.backend = backend
}

func (n *udpxNode) OnConfigure() {
	n.Node_.OnConfigure()
}
func (n *udpxNode) OnPrepare() {
	n.Node_.OnPrepare()
}

func (n *udpxNode) Maintain() { // runner
	n.Loop(time.Second, func(now time.Time) {
		// TODO: health check, markDown, markUp()
	})
	n.markDown()
	// TODO: wait for all conns
	if DebugLevel() >= 2 {
		Printf("udpxNode=%s done\n", n.name)
	}
	n.backend.DecSub() // node
}

func (n *udpxNode) dial() (*UConn, error) {
	// TODO. note: use n.IncSub()?
	return nil, nil
}

// poolUConn
var poolUConn sync.Pool

func getUConn(id int64, node *udpxNode, netConn net.PacketConn, rawConn syscall.RawConn) *UConn {
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
	// Conn states (non-zeros)
	id      int64 // the conn id
	node    *udpxNode
	netConn net.PacketConn
	rawConn syscall.RawConn // for syscall
	// Conn states (zeros)
	counter   atomic.Int64 // can be used to generate a random number
	lastWrite time.Time    // deadline of last write operation
	lastRead  time.Time    // deadline of last read operation
	broken    atomic.Bool  // is conn broken?
}

func (c *UConn) onGet(id int64, node *udpxNode, netConn net.PacketConn, rawConn syscall.RawConn) {
	c.id = id
	c.node = node
	c.netConn = netConn
	c.rawConn = rawConn
}
func (c *UConn) onPut() {
	c.netConn = nil
	c.rawConn = nil
	c.broken.Store(false)
	c.node = nil
	c.counter.Store(0)
	c.lastWrite = time.Time{}
	c.lastRead = time.Time{}
}

func (c *UConn) IsTLS() bool { return c.node.IsTLS() }
func (c *UConn) IsUDS() bool { return c.node.IsUDS() }

func (c *UConn) MakeTempName(p []byte, unixTime int64) int {
	return makeTempName(p, int64(c.node.backend.Stage().ID()), c.id, unixTime, c.counter.Add(1))
}

func (c *UConn) markBroken()    { c.broken.Store(true) }
func (c *UConn) isBroken() bool { return c.broken.Load() }

func (c *UConn) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	return c.netConn.WriteTo(p, addr)
}
func (c *UConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) { return c.netConn.ReadFrom(p) }

func (c *UConn) Close() error {
	// TODO: c.node.DecSub()?
	netConn := c.netConn
	putUConn(c)
	return netConn.Close()
}