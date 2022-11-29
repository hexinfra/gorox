// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// UDP/DTLS client implementation.

package internal

import (
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

func init() {
	registerFixture(signUDPS)
	registerBackend("udpsBackend", func(name string, stage *Stage) backend {
		b := new(UDPSBackend)
		b.onCreate(name, stage)
		return b
	})
}

// udpsClient is the interface for UDPSOutgate and UDPSBackend.
type udpsClient interface {
	client
}

// udpsClient_
type udpsClient_ struct {
	// Mixins
	// States
}

func (u *udpsClient_) onCreate() {
}

func (u *udpsClient_) onConfigure(c Component) {
}
func (u *udpsClient_) onPrepare(c Component) {
}

const signUDPS = "udps"

func createUDPS(stage *Stage) *UDPSOutgate {
	udps := new(UDPSOutgate)
	udps.onCreate(stage)
	udps.setShell(udps)
	return udps
}

// UDPSOutgate component.
type UDPSOutgate struct {
	// Mixins
	client_
	udpsClient_
	// States
}

func (f *UDPSOutgate) onCreate(stage *Stage) {
	f.client_.onCreate(signUDPS, stage)
	f.udpsClient_.onCreate()
}

func (f *UDPSOutgate) OnConfigure() {
	f.client_.onConfigure()
	f.udpsClient_.onConfigure(f)
}
func (f *UDPSOutgate) OnPrepare() {
	f.client_.onConfigure()
	f.udpsClient_.onPrepare(f)
}

func (f *UDPSOutgate) OnShutdown() {
	f.Shutdown()
}

func (f *UDPSOutgate) run() { // goroutine
	Loop(time.Second, f.Shut, func(now time.Time) {
		// TODO
	})
	if Debug(2) {
		fmt.Println("udps done")
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
	backend_[*udpsNode]
	udpsClient_
	loadBalancer_
	// States
	health any // TODO
}

func (b *UDPSBackend) onCreate(name string, stage *Stage) {
	b.backend_.onCreate(name, stage, b)
	b.udpsClient_.onCreate()
	b.loadBalancer_.init()
}

func (b *UDPSBackend) OnConfigure() {
	b.backend_.onConfigure()
	b.udpsClient_.onConfigure(b)
	b.loadBalancer_.onConfigure(b)
}
func (b *UDPSBackend) OnPrepare() {
	b.backend_.onPrepare()
	b.udpsClient_.onPrepare(b)
	b.loadBalancer_.onPrepare(len(b.nodes))
}

func (b *UDPSBackend) OnShutdown() {
	b.Shutdown()
}

func (b *UDPSBackend) createNode(id int32) *udpsNode {
	n := new(udpsNode)
	n.init(id, b)
	return n
}

func (b *UDPSBackend) Dial() (*UConn, error) {
	node := b.nodes[b.getIndex()]
	return node.dial()
}
func (b *UDPSBackend) FetchConn() (*UConn, error) {
	node := b.nodes[b.getIndex()]
	return node.fetchConn()
}
func (b *UDPSBackend) StoreConn(conn *UConn) {
	conn.node.storeConn(conn)
}

// udpsNode is a node in UDPSBackend.
type udpsNode struct {
	// Mixins
	node_
	// Assocs
	backend *UDPSBackend
	// States
}

func (n *udpsNode) init(id int32, backend *UDPSBackend) {
	n.node_.init(id)
	n.backend = backend
}

func (n *udpsNode) maintain(shut chan struct{}) { // goroutine
	Loop(time.Second, shut, func(now time.Time) {
		// TODO: health check
	})
	if Debug(2) {
		fmt.Printf("udpsNode=%d done\n", n.id)
	}
	n.backend.SubDone()
}

func (n *udpsNode) dial() (*UConn, error) {
	// TODO
	return nil, nil
}
func (n *udpsNode) fetchConn() (*UConn, error) {
	conn := n.takeConn()
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

func (c *UConn) isBroken() bool { return c.broken.Load() }
func (c *UConn) markBroken()    { c.broken.Store(true) }

func (c *UConn) SetWriteDeadline(deadline time.Time) error {
	if deadline.Sub(c.lastWrite) >= c.client.WriteTimeout()/4 {
		if err := c.udpConn.SetWriteDeadline(deadline); err != nil {
			return err
		}
		c.lastWrite = deadline
	}
	return nil
}
func (c *UConn) SetReadDeadline(deadline time.Time) error {
	if deadline.Sub(c.lastRead) >= c.client.ReadTimeout()/4 {
		if err := c.udpConn.SetReadDeadline(deadline); err != nil {
			return err
		}
		c.lastRead = deadline
	}
	return nil
}

func (c *UConn) Write(p []byte) (n int, err error) { return c.udpConn.Write(p) }
func (c *UConn) Read(p []byte) (n int, err error)  { return c.udpConn.Read(p) }

func (c *UConn) Close() error { // only used by clients of dial
	udpConn := c.udpConn
	putUConn(c)
	return udpConn.Close()
}

func (c *UConn) closeConn() { c.udpConn.Close() } // used by clients other than dial
