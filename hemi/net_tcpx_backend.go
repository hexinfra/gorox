// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// TCPX (TCP/TLS/UDS) backend.

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
	RegisterBackend("tcpxBackend", func(name string, stage *Stage) Backend {
		b := new(TCPXBackend)
		b.onCreate(name, stage)
		return b
	})
}

// TCPXBackend component.
type TCPXBackend struct {
	// Parent
	Backend_[*tcpxNode]
	// States
}

func (b *TCPXBackend) onCreate(name string, stage *Stage) {
	b.Backend_.OnCreate(name, stage)
}

func (b *TCPXBackend) OnConfigure() {
	b.Backend_.OnConfigure()

	// sub components
	b.ConfigureNodes()
}
func (b *TCPXBackend) OnPrepare() {
	b.Backend_.OnPrepare()

	// sub components
	b.PrepareNodes()
}

func (b *TCPXBackend) CreateNode(name string) Node {
	node := new(tcpxNode)
	node.onCreate(name, b)
	b.AddNode(node)
	return node
}

func (b *TCPXBackend) Dial() (*TConn, error) {
	node := b.nodes[b.nextIndex()]
	return node.dial()
}

// tcpxNode is a node in TCPXBackend.
type tcpxNode struct {
	// Parent
	Node_
	// Assocs
	backend *TCPXBackend
	// States
}

func (n *tcpxNode) onCreate(name string, backend *TCPXBackend) {
	n.Node_.OnCreate(name)
	n.backend = backend
}

func (n *tcpxNode) OnConfigure() {
	n.Node_.OnConfigure()
}
func (n *tcpxNode) OnPrepare() {
	n.Node_.OnPrepare()
}

func (n *tcpxNode) Maintain() { // runner
	n.Loop(time.Second, func(now time.Time) {
		// TODO: health check, markDown, markUp()
	})
	n.markDown()
	n.WaitSubs() // conns. TODO: max timeout?
	if DebugLevel() >= 2 {
		Printf("tcpxNode=%s done\n", n.name)
	}
	n.backend.DecSub() // node
}

func (n *tcpxNode) dial() (*TConn, error) {
	if DebugLevel() >= 2 {
		Printf("tcpxNode=%s dial %s\n", n.name, n.address)
	}
	var (
		tConn *TConn
		err   error
	)
	if n.IsTLS() {
		tConn, err = n._dialTLS()
	} else if n.IsUDS() {
		tConn, err = n._dialUDS()
	} else {
		tConn, err = n._dialTCP()
	}
	if err != nil {
		return nil, errNodeDown
	}
	n.IncSub() // conn
	return tConn, err
}
func (n *tcpxNode) _dialTLS() (*TConn, error) {
	// TODO: dynamic address names?
	netConn, err := net.DialTimeout("tcp", n.address, n.backend.DialTimeout())
	if err != nil {
		n.markDown()
		return nil, err
	}
	if DebugLevel() >= 2 {
		Printf("tcpxNode=%s dial %s OK!\n", n.name, n.address)
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
func (n *tcpxNode) _dialUDS() (*TConn, error) {
	// TODO: dynamic address names?
	netConn, err := net.DialTimeout("unix", n.address, n.backend.DialTimeout())
	if err != nil {
		n.markDown()
		return nil, err
	}
	if DebugLevel() >= 2 {
		Printf("tcpxNode=%s dial %s OK!\n", n.name, n.address)
	}
	connID := n.backend.nextConnID()
	rawConn, err := netConn.(*net.UnixConn).SyscallConn()
	if err != nil {
		netConn.Close()
		return nil, err
	}
	return getTConn(connID, n, netConn, rawConn), nil
}
func (n *tcpxNode) _dialTCP() (*TConn, error) {
	// TODO: dynamic address names?
	netConn, err := net.DialTimeout("tcp", n.address, n.backend.DialTimeout())
	if err != nil {
		n.markDown()
		return nil, err
	}
	if DebugLevel() >= 2 {
		Printf("tcpxNode=%s dial %s OK!\n", n.name, n.address)
	}
	connID := n.backend.nextConnID()
	rawConn, err := netConn.(*net.TCPConn).SyscallConn()
	if err != nil {
		netConn.Close()
		return nil, err
	}
	return getTConn(connID, n, netConn, rawConn), nil
}

// poolTConn
var poolTConn sync.Pool

func getTConn(id int64, node *tcpxNode, netConn net.Conn, rawConn syscall.RawConn) *TConn {
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

// TConn is a backend-side connection to tcpxNode.
type TConn struct {
	// Conn states (stocks)
	stockInput [8192]byte // for c.input
	// Conn states (controlled)
	// Conn states (non-zeros)
	id        int64     // the conn id
	expire    time.Time // when the conn is considered expired
	node      *tcpxNode
	netConn   net.Conn        // *net.TCPConn, *tls.Conn, *net.UnixConn
	rawConn   syscall.RawConn // for syscall. only usable when netConn is TCP/UDS
	input     []byte
	closeSema atomic.Int32 // for close read and close write
	// Conn states (zeros)
	counter     atomic.Int64 // can be used to generate a random number
	lastWrite   time.Time    // deadline of last write operation
	lastRead    time.Time    // deadline of last read operation
	writeBroken atomic.Bool  // write-side broken?
	readBroken  atomic.Bool  // read-side broken?
}

func (c *TConn) onGet(id int64, node *tcpxNode, netConn net.Conn, rawConn syscall.RawConn) {
	c.id = id
	c.expire = time.Now().Add(node.backend.aliveTimeout)
	c.node = node
	c.netConn = netConn
	c.rawConn = rawConn
	c.input = c.stockInput[:]
	c.closeSema.Store(2) // read and write
}
func (c *TConn) onPut() {
	c.input = nil
	c.netConn = nil
	c.rawConn = nil
	c.node = nil

	c.writeBroken.Store(false)
	c.readBroken.Store(false)
	c.expire = time.Time{}
	c.counter.Store(0)
	c.lastWrite = time.Time{}
	c.lastRead = time.Time{}
}

func (c *TConn) IsTLS() bool { return c.node.IsTLS() }
func (c *TConn) IsUDS() bool { return c.node.IsUDS() }

func (c *TConn) MakeTempName(p []byte, unixTime int64) int {
	return makeTempName(p, int64(c.node.backend.Stage().ID()), c.id, unixTime, c.counter.Add(1))
}

//func (c *TConn) isAlive() bool { return time.Now().Before(c.expire) }

func (c *TConn) proxyPass(conn *TCPXConn) error {
	var (
		p   []byte
		err error
	)
	for {
		p, err = conn.Recv()
		if err != nil {
			return err
		}
		if p == nil {
			if err = c.CloseWrite(); err != nil {
				c.markWriteBroken()
				return err
			}
			return nil
		}
		if _, err = c.Write(p); err != nil {
			c.markWriteBroken()
			return err
		}
	}
}

func (c *TConn) MarkBroken() {
	c.markWriteBroken()
	c.markReadBroken()
}
func (c *TConn) markWriteBroken() { c.writeBroken.Store(true) }
func (c *TConn) markReadBroken()  { c.readBroken.Store(true) }
func (c *TConn) IsBroken() bool   { return c.writeBroken.Load() || c.readBroken.Load() }

func (c *TConn) setWriteDeadline(deadline time.Time) error {
	if deadline.Sub(c.lastWrite) >= time.Second {
		if err := c.netConn.SetWriteDeadline(deadline); err != nil {
			return err
		}
		c.lastWrite = deadline
	}
	return nil
}
func (c *TConn) setReadDeadline(deadline time.Time) error {
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

func (c *TConn) CloseWrite() error {
	if c.IsTLS() {
		return c.netConn.(*tls.Conn).CloseWrite()
	} else if c.IsUDS() {
		return c.netConn.(*net.UnixConn).CloseWrite()
	} else {
		return c.netConn.(*net.TCPConn).CloseWrite()
	}
}

func (c *TConn) Close() error {
	c.node.DecSub() // conn
	netConn := c.netConn
	putTConn(c)
	return netConn.Close()
}
