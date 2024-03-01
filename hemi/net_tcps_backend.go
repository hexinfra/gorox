// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// TCPS (TCP/TLS/UDS) backend.

package hemi

import (
	"crypto/tls"
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
	_streamHolder_
	_loadBalancer_
	// States
	health any // TODO
}

func (b *TCPSBackend) onCreate(name string, stage *Stage) {
	b.Backend_.OnCreate(name, stage, b.NewNode)
	b._loadBalancer_.init()
}

func (b *TCPSBackend) OnConfigure() {
	b.Backend_.OnConfigure()
	b._streamHolder_.onConfigure(b, 1000)
	b._loadBalancer_.onConfigure(b)
}
func (b *TCPSBackend) OnPrepare() {
	b.Backend_.OnPrepare()
	b._streamHolder_.onPrepare(b)
	b._loadBalancer_.onPrepare(len(b.nodes))
}

func (b *TCPSBackend) NewNode(id int32) *tcpsNode {
	node := new(tcpsNode)
	node.init(id, b)
	return node
}

func (b *TCPSBackend) FetchConn() (*TConn, error) {
	node := b.nodes[b.getNext()]
	return node.fetchConn()
}
func (b *TCPSBackend) StoreConn(tConn *TConn) {
	tConn.node.(*tcpsNode).storeConn(tConn)
}

func (b *TCPSBackend) Dial() (*TConn, error) {
	node := b.nodes[b.getNext()]
	return node.dial()
}

// tcpsNode is a node in TCPSBackend.
type tcpsNode struct {
	// Parent
	Node_
	// Assocs
	// States
}

func (n *tcpsNode) init(id int32, backend *TCPSBackend) {
	n.Node_.Init(id, backend)
}

func (n *tcpsNode) Maintain() { // runner
	n.Loop(time.Second, func(now time.Time) {
		// TODO: health check, markUp()
	})
	n.markDown()
	if size := n.closeFree(); size > 0 {
		n.SubsAddn(-size)
	}
	n.WaitSubs() // conns
	if Debug() >= 2 {
		Printf("tcpsNode=%d done\n", n.id)
	}
	n.backend.DecSub()
}

func (n *tcpsNode) fetchConn() (*TConn, error) {
	conn := n.pullConn()
	down := n.isDown()
	if conn != nil {
		tConn := conn.(*TConn)
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
		if Debug() >= 2 {
			Printf("TConn[node=%d id=%d] closed\n", tConn.node.ID(), tConn.id)
		}
		n.closeConn(tConn)
	} else {
		if Debug() >= 2 {
			Printf("TConn[node=%d id=%d] pushed\n", tConn.node.ID(), tConn.id)
		}
		n.pushConn(tConn)
	}
}

func (n *tcpsNode) dial() (*TConn, error) { // some protocols don't support or need connection reusing, just dial & tConn.close.
	if Debug() >= 2 {
		Printf("tcpsNode=%d dial %s\n", n.id, n.address)
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
	if Debug() >= 2 {
		Printf("tcpsNode=%d dial %s OK!\n", n.id, n.address)
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
	if Debug() >= 2 {
		Printf("tcpsNode=%d dial %s OK!\n", n.id, n.address)
	}
	connID := n.backend.nextConnID()
	rawConn, err := netConn.(*net.TCPConn).SyscallConn()
	if err != nil {
		netConn.Close()
		return nil, err
	}
	return getTConn(connID, n, netConn, rawConn), nil
}

func (n *tcpsNode) closeConn(tConn *TConn) {
	tConn.Close()
	n.DecSub()
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
	// Conn states (non-zeros)
	netConn    net.Conn        // *net.TCPConn, *tls.Conn, *net.UnixConn
	rawConn    syscall.RawConn // for syscall. only usable when netConn is TCP/UDS
	maxStreams int32           // how many streams are allowed on this conn?
	// Conn states (zeros)
	counter     atomic.Int64 // used to make temp name
	usedStreams atomic.Int32 // how many streams has been used?
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
	c.counter.Store(0)
	c.usedStreams.Store(0)
	c.writeBroken.Store(false)
	c.readBroken.Store(false)
	c.BackendConn_.OnPut()
}

func (c *TConn) TCPConn() *net.TCPConn  { return c.netConn.(*net.TCPConn) }
func (c *TConn) TLSConn() *tls.Conn     { return c.netConn.(*tls.Conn) }
func (c *TConn) UDSConn() *net.UnixConn { return c.netConn.(*net.UnixConn) }

func (c *TConn) reachLimit() bool { return c.usedStreams.Add(1) > c.maxStreams }

func (c *TConn) MakeTempName(p []byte, unixTime int64) int {
	return makeTempName(p, int64(c.backend.Stage().ID()), c.id, unixTime, c.counter.Add(1))
}

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
