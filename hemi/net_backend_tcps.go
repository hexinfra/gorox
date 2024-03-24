// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// TCPS (TCP/TLS/UDS) backend.

package hemi

import (
	"crypto/tls"
	"errors"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

func init() {
	RegisterBackend("tcpsBackend", func(name string, stage *Stage) Backend {
		b := new(TCPSBackend)
		b.onCreate(name, stage)
		return b
	})
}

// TCPSBackend component.
type TCPSBackend struct {
	// Parent
	Backend_[*tcpsNode]
	// Mixins
	// States
	maxStreamsPerConn int32 // max streams of one conn. 0 means infinite
}

func (b *TCPSBackend) onCreate(name string, stage *Stage) {
	b.Backend_.OnCreate(name, stage)
}

func (b *TCPSBackend) OnConfigure() {
	b.Backend_.OnConfigure()

	// maxStreamsPerConn
	b.ConfigureInt32("maxStreamsPerConn", &b.maxStreamsPerConn, func(value int32) error {
		if value >= 0 {
			return nil
		}
		return errors.New(".maxStreamsPerConn has an invalid value")
	}, 1000)

	// sub components
	b.ConfigureNodes()
}
func (b *TCPSBackend) OnPrepare() {
	b.Backend_.OnPrepare()

	// sub components
	b.PrepareNodes()
}

func (b *TCPSBackend) MaxStreamsPerConn() int32 { return b.maxStreamsPerConn }

func (b *TCPSBackend) CreateNode(name string) Node {
	node := new(tcpsNode)
	node.onCreate(name, b)
	b.AddNode(node)
	return node
}

func (b *TCPSBackend) Dial() (*TConn, error) {
	node := b.nodes[b.nextIndex()]
	return node.dial()
}

func (b *TCPSBackend) FetchConn() (*TConn, error) {
	node := b.nodes[b.nextIndex()]
	return node.fetchConn()
}
func (b *TCPSBackend) StoreConn(tConn *TConn) {
	tConn.node.(*tcpsNode).storeConn(tConn)
}

// tcpsNode is a node in TCPSBackend.
type tcpsNode struct {
	// Parent
	Node_
	// Assocs
	// States
	connPool struct { // free list of conns in this node
		sync.Mutex
		head *TConn // head element
		tail *TConn // tail element
		qnty int    // size of the list
	}
}

func (n *tcpsNode) onCreate(name string, backend *TCPSBackend) {
	n.Node_.OnCreate(name, backend)
}

func (n *tcpsNode) OnConfigure() {
	n.Node_.OnConfigure()
}
func (n *tcpsNode) OnPrepare() {
	n.Node_.OnPrepare()
}

func (n *tcpsNode) Maintain() { // runner
	n.Loop(time.Second, func(now time.Time) {
		// TODO: health check, markDown, markUp()
	})
	n.markDown()
	if size := n.closeFree(); size > 0 {
		n.SubsAddn(-size)
	}
	n.WaitSubs() // conns
	if DbgLevel() >= 2 {
		Printf("tcpsNode=%s done\n", n.name)
	}
	n.backend.DecSub()
}

func (n *tcpsNode) dial() (*TConn, error) { // some protocols don't support or need connection reusing, just dial & tConn.close.
	if DbgLevel() >= 2 {
		Printf("tcpsNode=%s dial %s\n", n.name, n.address)
	}
	if n.IsUDS() {
		return n._dialUDS()
	} else if n.IsTLS() {
		return n._dialTLS()
	} else {
		return n._dialTCP()
	}
}
func (n *tcpsNode) _dialUDS() (*TConn, error) {
	// TODO
	return nil, nil
}
func (n *tcpsNode) _dialTLS() (*TConn, error) {
	// TODO: dynamic address names?
	netConn, err := net.DialTimeout("tcp", n.address, n.backend.DialTimeout())
	if err != nil {
		n.markDown()
		return nil, err
	}
	if DbgLevel() >= 2 {
		Printf("tcpsNode=%s dial %s OK!\n", n.name, n.address)
	}
	connID := n.backend.nextConnID()
	tlsConn := tls.Client(netConn, n.tlsConfig)
	if err := tlsConn.SetDeadline(time.Now().Add(10 * time.Second)); err != nil {
		tlsConn.Close()
		return nil, err
	}
	if err := tlsConn.Handshake(); err != nil {
		tlsConn.Close()
		return nil, err
	}
	return getTConn(connID, n, tlsConn, nil), nil
}
func (n *tcpsNode) _dialTCP() (*TConn, error) {
	// TODO: dynamic address names?
	netConn, err := net.DialTimeout("tcp", n.address, n.backend.DialTimeout())
	if err != nil {
		n.markDown()
		return nil, err
	}
	if DbgLevel() >= 2 {
		Printf("tcpsNode=%s dial %s OK!\n", n.name, n.address)
	}
	connID := n.backend.nextConnID()
	rawConn, err := netConn.(*net.TCPConn).SyscallConn()
	if err != nil {
		netConn.Close()
		return nil, err
	}
	return getTConn(connID, n, netConn, rawConn), nil
}

func (n *tcpsNode) fetchConn() (*TConn, error) {
	tConn := n.pullConn()
	down := n.isDown()
	if tConn != nil {
		if tConn.isAlive() && !tConn.reachLimit() && !down {
			return tConn, nil
		}
		n.closeConn(tConn)
	}
	if down {
		return nil, errNodeDown
	}
	tConn, err := n.dial()
	if err == nil {
		n.IncSub()
	}
	return tConn, err
}
func (n *tcpsNode) storeConn(tConn *TConn) {
	if tConn.IsBroken() || n.isDown() || !tConn.isAlive() {
		if DbgLevel() >= 2 {
			Printf("TConn[node=%s id=%d] closed\n", tConn.node.Name(), tConn.id)
		}
		n.closeConn(tConn)
	} else {
		if DbgLevel() >= 2 {
			Printf("TConn[node=%s id=%d] pushed\n", tConn.node.Name(), tConn.id)
		}
		n.pushConn(tConn)
	}
}

func (n *tcpsNode) pullConn() *TConn {
	list := &n.connPool

	list.Lock()
	defer list.Unlock()

	if list.qnty == 0 {
		return nil
	}
	conn := list.head
	list.head = conn.next
	conn.next = nil
	list.qnty--

	return conn
}
func (n *tcpsNode) pushConn(conn *TConn) {
	list := &n.connPool

	list.Lock()
	defer list.Unlock()

	if list.qnty == 0 {
		list.head = conn
		list.tail = conn
	} else { // >= 1
		list.tail.next = conn
		list.tail = conn
	}
	list.qnty++
}
func (n *tcpsNode) closeFree() int {
	list := &n.connPool

	list.Lock()
	defer list.Unlock()

	for conn := list.head; conn != nil; conn = conn.next {
		conn.Close()
	}
	qnty := list.qnty
	list.qnty = 0
	list.head, list.tail = nil, nil

	return qnty
}

// poolTConn
var poolTConn sync.Pool

func getTConn(id int64, node *tcpsNode, netConn net.Conn, rawConn syscall.RawConn) *TConn {
	var tConn *TConn
	if x := poolTConn.Get(); x == nil {
		tConn = new(TConn)
	} else {
		tConn = x.(*TConn)
	}
	tConn.onGet(id, node, netConn, rawConn)
	return tConn
}
func putTConn(tConn *TConn) {
	tConn.onPut()
	poolTConn.Put(tConn)
}

// TConn is a backend-side connection to tcpsNode.
type TConn struct {
	// Parent
	BackendConn_
	// Assocs
	next *TConn // the linked-list
	// Conn states (non-zeros)
	netConn    net.Conn        // *net.TCPConn, *tls.Conn, *net.UnixConn
	rawConn    syscall.RawConn // for syscall. only usable when netConn is TCP/UDS
	maxStreams int32           // how many streams are allowed on this conn?
	// Conn states (zeros)
	usedStreams atomic.Int32 // how many streams have been used?
	writeBroken atomic.Bool  // write-side broken?
	readBroken  atomic.Bool  // read-side broken?
}

func (c *TConn) onGet(id int64, node *tcpsNode, netConn net.Conn, rawConn syscall.RawConn) {
	c.BackendConn_.OnGet(id, node)
	c.netConn = netConn
	c.rawConn = rawConn
	c.maxStreams = node.Backend().(*TCPSBackend).MaxStreamsPerConn()
}
func (c *TConn) onPut() {
	c.netConn = nil
	c.rawConn = nil
	c.usedStreams.Store(0)
	c.writeBroken.Store(false)
	c.readBroken.Store(false)
	c.BackendConn_.OnPut()
}

func (c *TConn) TCPConn() *net.TCPConn  { return c.netConn.(*net.TCPConn) }
func (c *TConn) TLSConn() *tls.Conn     { return c.netConn.(*tls.Conn) }
func (c *TConn) UDSConn() *net.UnixConn { return c.netConn.(*net.UnixConn) }

func (c *TConn) reachLimit() bool { return c.usedStreams.Add(1) > c.maxStreams }

func (c *TConn) SetWriteDeadline(deadline time.Time) error {
	if deadline.Sub(c.lastWrite) >= time.Second {
		if err := c.netConn.SetWriteDeadline(deadline); err != nil {
			return err
		}
		c.lastWrite = deadline
	}
	return nil
}
func (c *TConn) SetReadDeadline(deadline time.Time) error {
	if deadline.Sub(c.lastRead) >= time.Second {
		if err := c.netConn.SetReadDeadline(deadline); err != nil {
			return err
		}
		c.lastRead = deadline
	}
	return nil
}

func (c *TConn) Write(p []byte) (n int, err error)         { return c.netConn.Write(p) }
func (c *TConn) Writev(vector *net.Buffers) (int64, error) { return vector.WriteTo(c.netConn) }
func (c *TConn) Read(p []byte) (n int, err error)          { return c.netConn.Read(p) }
func (c *TConn) ReadFull(p []byte) (n int, err error)      { return io.ReadFull(c.netConn, p) }
func (c *TConn) ReadAtLeast(p []byte, min int) (n int, err error) {
	return io.ReadAtLeast(c.netConn, p, min)
}

func (c *TConn) IsBroken() bool { return c.writeBroken.Load() || c.readBroken.Load() }
func (c *TConn) MarkBroken() {
	c.markWriteBroken()
	c.markReadBroken()
}

func (c *TConn) markWriteBroken() { c.writeBroken.Store(true) }
func (c *TConn) markReadBroken()  { c.readBroken.Store(true) }

func (c *TConn) CloseWrite() error {
	if c.IsUDS() {
		return c.netConn.(*net.UnixConn).CloseWrite()
	} else if c.IsTLS() {
		return c.netConn.(*tls.Conn).CloseWrite()
	} else {
		return c.netConn.(*net.TCPConn).CloseWrite()
	}
}

func (c *TConn) Close() error {
	netConn := c.netConn
	putTConn(c)
	return netConn.Close()
}
