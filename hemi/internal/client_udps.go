// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// UDP/DTLS client implementation.

package internal

import (
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

func init() {
	registerFixture(signUDPSOutgate)
	RegisterBackend("udpsBackend", func(name string, stage *Stage) Backend {
		b := new(UDPSBackend)
		b.onCreate(name, stage)
		return b
	})
}

// udpsClient is the interface for UDPSOutgate and UDPSBackend.
type udpsClient interface {
	client
}

const signUDPSOutgate = "udpsOutgate"

func createUDPSOutgate(stage *Stage) *UDPSOutgate {
	udps := new(UDPSOutgate)
	udps.onCreate(stage)
	udps.setShell(udps)
	return udps
}

// UDPSOutgate component.
type UDPSOutgate struct {
	// Mixins
	outgate_
	// States
}

func (f *UDPSOutgate) onCreate(stage *Stage) {
	f.outgate_.onCreate(signUDPSOutgate, stage)
}

func (f *UDPSOutgate) OnConfigure() {
	f.outgate_.onConfigure()
}
func (f *UDPSOutgate) OnPrepare() {
	f.outgate_.onConfigure()
}

func (f *UDPSOutgate) run() { // goroutine
	f.Loop(time.Second, func(now time.Time) {
		// TODO
	})
	if IsDebug(2) {
		Debugln("udpsOutgate done")
	}
	f.stage.SubDone()
}

func (f *UDPSOutgate) Dial(address string, tlsMode bool) (*UConn, error) {
	// TODO
	return nil, nil
}
func (f *UDPSOutgate) FetchConn(address string, tlsMode bool) (*UConn, error) {
	// TODO
	return nil, nil
}
func (f *UDPSOutgate) StoreConn(conn *UConn) {
	// TODO
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

func (b *UDPSBackend) Dial() (*UConn, error) {
	node := b.nodes[b.getNext()]
	return node.dial()
}
func (b *UDPSBackend) FetchConn() (*UConn, error) {
	node := b.nodes[b.getNext()]
	return node.fetchConn()
}
func (b *UDPSBackend) StoreConn(conn *UConn) {
	conn.node.storeConn(conn)
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

func (n *udpsNode) Maintain() { // goroutine
	n.Loop(time.Second, func(now time.Time) {
		// TODO: health check
	})
	// TODO: wait for all conns
	if IsDebug(2) {
		Debugf("udpsNode=%d done\n", n.id)
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
		uConn.closeConn()
		putUConn(uConn)
	}
	return n.dial()
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

func getUConn(id int64, client udpsClient, node *udpsNode, udpConn *net.UDPConn, rawConn syscall.RawConn) *UConn {
	var conn *UConn
	if x := poolUConn.Get(); x == nil {
		conn = new(UConn)
	} else {
		conn = x.(*UConn)
	}
	conn.onGet(id, client, node, udpConn, rawConn)
	return conn
}
func putUConn(conn *UConn) {
	conn.onPut()
	poolUConn.Put(conn)
}

// UConn is a client-side connection to udpsNode.
type UConn struct { // only exported to hemi
	// Mixins
	conn_
	// Conn states (non-zeros)
	node    *udpsNode       // associated node if client is UDPSBackend
	udpConn *net.UDPConn    // udp conn
	rawConn syscall.RawConn // for syscall
	// Conn states (zeros)
	broken atomic.Bool // is conn broken?
}

func (c *UConn) onGet(id int64, client udpsClient, node *udpsNode, udpConn *net.UDPConn, rawConn syscall.RawConn) {
	c.conn_.onGet(id, client)
	c.node = node
	c.udpConn = udpConn
	c.rawConn = rawConn
}
func (c *UConn) onPut() {
	c.conn_.onPut()
	c.node = nil
	c.udpConn = nil
	c.rawConn = nil
	c.broken.Store(false)
}

func (c *UConn) getClient() udpsClient { return c.client.(udpsClient) }

func (c *UConn) SetWriteDeadline(deadline time.Time) error {
	if deadline.Sub(c.lastWrite) >= time.Second {
		if err := c.udpConn.SetWriteDeadline(deadline); err != nil {
			return err
		}
		c.lastWrite = deadline
	}
	return nil
}
func (c *UConn) SetReadDeadline(deadline time.Time) error {
	if deadline.Sub(c.lastRead) >= time.Second {
		if err := c.udpConn.SetReadDeadline(deadline); err != nil {
			return err
		}
		c.lastRead = deadline
	}
	return nil
}

func (c *UConn) Write(p []byte) (n int, err error) { return c.udpConn.Write(p) }
func (c *UConn) Read(p []byte) (n int, err error)  { return c.udpConn.Read(p) }

func (c *UConn) isBroken() bool { return c.broken.Load() }
func (c *UConn) markBroken()    { c.broken.Store(true) }

func (c *UConn) Close() error { // only used by clients of dial
	udpConn := c.udpConn
	putUConn(c)
	return udpConn.Close()
}

func (c *UConn) closeConn() { c.udpConn.Close() } // used by codes other than dial
