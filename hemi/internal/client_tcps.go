// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// TCP/TLS client implementation.

package internal

import (
	"crypto/tls"
	"github.com/hexinfra/gorox/hemi/libraries/system"
	"io"
	"net"
	"sync"
	"syscall"
	"time"
)

// tcpsClient is the interface for TCPSOutgate and TCPSBackend.
type tcpsClient interface {
	client
	streamHolder
}

func init() {
	registerBackend("tcpsBackend", func(name string, stage *Stage) backend {
		b := new(TCPSBackend)
		b.init(name, stage)
		return b
	})
}

// TCPSBackend component.
type TCPSBackend struct {
	// Mixins
	backend_
	streamHolder_
	// States
	healthCheck any         // TODO
	nodes       []*tcpsNode // nodes of backend
}

func (b *TCPSBackend) init(name string, stage *Stage) {
	b.backend_.init(name, stage)
}

func (b *TCPSBackend) OnConfigure() {
	b.configure()
	// maxStreamsPerConn
	b.ConfigureInt32("maxStreamsPerConn", &b.maxStreamsPerConn, func(value int32) bool { return value > 0 }, 1000)
}
func (b *TCPSBackend) OnPrepare() {
	b.prepare(len(b.nodes))
}
func (b *TCPSBackend) OnShutdown() {
	b.shutdown()
}

func (b *TCPSBackend) maintain() { // blocking
	// TODO: health check for all nodes
}

func (b *TCPSBackend) Dial() (PConn, error) {
	node := b.nodes[b.getIndex()]
	return node.dial()
}
func (b *TCPSBackend) FetchConn() (PConn, error) {
	node := b.nodes[b.getIndex()]
	return node.fetchConn()
}
func (b *TCPSBackend) StoreConn(conn PConn) {
	tConn := conn.(*TConn)
	tConn.node.storeConn(tConn)
}

// tcpsNode is a node in TCPSBackend.
type tcpsNode struct {
	// Mixins
	node_
	// Assocs
	backend *TCPSBackend
	// States
}

func (n *tcpsNode) init(id int32, backend *TCPSBackend) {
	n.node_.init(id)
	n.backend = backend
}

func (n *tcpsNode) checkHealth() {
	// TODO
	for {
		time.Sleep(time.Second)
	}
}

func (n *tcpsNode) dial() (*TConn, error) { // some protocols don't support or need connection reusing, just dial & close.
	netConn, err := net.DialTimeout("tcp", n.address, n.backend.dialTimeout)
	if err != nil {
		n.markDown()
		return nil, err
	}
	connID := n.backend.nextConnID()
	if n.backend.tlsMode {
		tlsConn := tls.Client(netConn, n.backend.tlsConfig)
		// TODO: timeout
		if err := tlsConn.Handshake(); err != nil {
			tlsConn.Close()
			return nil, err
		}
		return getTConn(connID, n.backend, n, tlsConn, nil), nil
	} else {
		rawConn, err := netConn.(*net.TCPConn).SyscallConn()
		if err != nil {
			netConn.Close()
			return nil, err
		}
		return getTConn(connID, n.backend, n, netConn, rawConn), nil
	}
}
func (n *tcpsNode) fetchConn() (*TConn, error) {
	conn := n.takeConn()
	down := n.isDown()
	if conn != nil {
		tConn := conn.(*TConn)
		if tConn.isAlive() && !tConn.reachLimit() && !down {
			return tConn, nil
		}
		tConn.closeConn()
		putTConn(tConn)
	}
	if down {
		return nil, errNodeDown
	}
	return n.dial()
}
func (n *tcpsNode) storeConn(tConn *TConn) {
	if tConn.isBroken() || n.isDown() || !tConn.isAlive() {
		tConn.closeConn()
		putTConn(tConn)
	} else {
		n.pushConn(tConn)
	}
}

// poolTConn
var poolTConn sync.Pool

func getTConn(id int64, client tcpsClient, node *tcpsNode, netConn net.Conn, rawConn syscall.RawConn) *TConn {
	var conn *TConn
	if x := poolTConn.Get(); x == nil {
		conn = new(TConn)
	} else {
		conn = x.(*TConn)
	}
	conn.onGet(id, client, node, netConn, rawConn)
	return conn
}
func putTConn(conn *TConn) {
	conn.onPut()
	poolTConn.Put(conn)
}

// TConn is a client-side connection to tcpsNode.
type TConn struct { // only exported to hemi
	// Mixins
	pConn_
	// Conn states (non-zeros)
	node    *tcpsNode       // belonging node if client is TCPSBackend
	netConn net.Conn        // TCP, TLS
	rawConn syscall.RawConn // for syscall. only usable when netConn is TCP
	// Conn states (zeros)
}

func (c *TConn) onGet(id int64, client tcpsClient, node *tcpsNode, netConn net.Conn, rawConn syscall.RawConn) {
	c.pConn_.onGet(id, client, client.MaxStreamsPerConn())
	c.node = node
	c.netConn = netConn
	c.rawConn = rawConn
}
func (c *TConn) onPut() {
	c.pConn_.onPut()
	c.node = nil
	c.netConn = nil
	c.rawConn = nil
}

func (c *TConn) getClient() tcpsClient { return c.client.(tcpsClient) }

func (c *TConn) TCPConn() *net.TCPConn { return c.netConn.(*net.TCPConn) }
func (c *TConn) TLSConn() *tls.Conn    { return c.netConn.(*tls.Conn) }

func (c *TConn) SetBuffered(buffered bool) {
	if !c.client.TLSMode() { // we can only call rawConn on TCP.
		system.SetBuffered(c.rawConn, buffered)
	}
}
func (c *TConn) SetWriteDeadline(deadline time.Time) error {
	if deadline.Sub(c.lastWrite) >= c.client.WriteTimeout()/4 {
		if err := c.netConn.SetWriteDeadline(deadline); err != nil {
			return err
		}
		c.lastWrite = deadline
	}
	return nil
}
func (c *TConn) SetReadDeadline(deadline time.Time) error {
	if deadline.Sub(c.lastRead) >= c.client.ReadTimeout()/4 {
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

func (c *TConn) CloseWrite() error { return c.netConn.(*net.TCPConn).CloseWrite() }
func (c *TConn) CloseRead() error  { return c.netConn.(*net.TCPConn).CloseRead() }

func (c *TConn) Close() error { // only used by clients of dial
	netConn := c.netConn
	putTConn(c)
	return netConn.Close()
}

func (c *TConn) closeConn() { c.netConn.Close() } // used by clients other than dial
