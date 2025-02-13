// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// TCPX (TCP/TLS/UDS) backend implementation. See RFC 9293.

package hemi

import (
	"crypto/tls"
	"net"
	"sync"
	"syscall"
	"time"
)

func init() {
	RegisterBackend("tcpxBackend", func(compName string, stage *Stage) Backend {
		b := new(TCPXBackend)
		b.onCreate(compName, stage)
		return b
	})
}

// TCPXBackend component.
type TCPXBackend struct {
	// Parent
	Backend_[*tcpxNode]
	// States
}

func (b *TCPXBackend) onCreate(compName string, stage *Stage) {
	b.Backend_.OnCreate(compName, stage)
}

func (b *TCPXBackend) OnConfigure() {
	b.Backend_.OnConfigure()
	b.ConfigureNodes()
}
func (b *TCPXBackend) OnPrepare() {
	b.Backend_.OnPrepare()
	b.PrepareNodes()
}

func (b *TCPXBackend) CreateNode(compName string) Node {
	node := new(tcpxNode)
	node.onCreate(compName, b.stage, b)
	b.AddNode(node)
	return node
}

func (b *TCPXBackend) Dial() (*TConn, error) {
	node := b.nodes[b.nodeIndexGet()]
	return node.dial()
}

// tcpxNode is a node in TCPXBackend.
type tcpxNode struct {
	// Parent
	Node_[*TCPXBackend]
	// Mixins
	_tcpxHolder_
	// States
}

func (n *tcpxNode) onCreate(compName string, stage *Stage, backend *TCPXBackend) {
	n.Node_.OnCreate(compName, stage, backend)
}

func (n *tcpxNode) OnConfigure() {
	n.Node_.OnConfigure()
	n._tcpxHolder_.onConfigure(n)
}
func (n *tcpxNode) OnPrepare() {
	n.Node_.OnPrepare()
	n._tcpxHolder_.onPrepare(n)
}

func (n *tcpxNode) Maintain() { // runner
	n.LoopRun(time.Second, func(now time.Time) {
		// TODO: health check, markDown, markUp()
	})
	n.markDown()
	n.WaitSubConns() // TODO: max timeout?
	if DebugLevel() >= 2 {
		Printf("tcpxNode=%s done\n", n.compName)
	}
	n.backend.DecNode()
}

func (n *tcpxNode) dial() (*TConn, error) {
	if DebugLevel() >= 2 {
		Printf("tcpxNode=%s dial %s\n", n.compName, n.address)
	}
	var (
		conn *TConn
		err  error
	)
	if n.UDSMode() {
		conn, err = n._dialUDS()
	} else if n.TLSMode() {
		conn, err = n._dialTLS()
	} else {
		conn, err = n._dialTCP()
	}
	if err != nil {
		return nil, errNodeDown
	}
	n.IncSubConn()
	return conn, err
}
func (n *tcpxNode) _dialUDS() (*TConn, error) {
	// TODO: dynamic address names?
	netConn, err := net.DialTimeout("unix", n.address, n.DialTimeout())
	if err != nil {
		n.markDown()
		return nil, err
	}
	if DebugLevel() >= 2 {
		Printf("tcpxNode=%s dial %s OK!\n", n.compName, n.address)
	}
	connID := n.nextConnID()
	rawConn, err := netConn.(*net.UnixConn).SyscallConn()
	if err != nil {
		netConn.Close()
		return nil, err
	}
	return getTConn(connID, n, netConn, rawConn), nil
}
func (n *tcpxNode) _dialTLS() (*TConn, error) {
	// TODO: dynamic address names?
	netConn, err := net.DialTimeout("tcp", n.address, n.DialTimeout())
	if err != nil {
		// TODO: handle ephemeral port exhaustion
		n.markDown()
		return nil, err
	}
	if DebugLevel() >= 2 {
		Printf("tcpxNode=%s dial %s OK!\n", n.compName, n.address)
	}
	connID := n.nextConnID()
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
func (n *tcpxNode) _dialTCP() (*TConn, error) {
	// TODO: dynamic address names?
	netConn, err := net.DialTimeout("tcp", n.address, n.DialTimeout())
	if err != nil {
		// TODO: handle ephemeral port exhaustion
		n.markDown()
		return nil, err
	}
	if DebugLevel() >= 2 {
		Printf("tcpxNode=%s dial %s OK!\n", n.compName, n.address)
	}
	connID := n.nextConnID()
	rawConn, err := netConn.(*net.TCPConn).SyscallConn()
	if err != nil {
		netConn.Close()
		return nil, err
	}
	return getTConn(connID, n, netConn, rawConn), nil
}

// TConn is a backend-side connection to tcpxNode.
type TConn struct {
	// Parent
	tcpxConn_
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	node *tcpxNode // the node to which the connection belongs
	// Conn states (zeros)
}

var poolTConn sync.Pool

func getTConn(id int64, node *tcpxNode, netConn net.Conn, rawConn syscall.RawConn) *TConn {
	var conn *TConn
	if x := poolTConn.Get(); x == nil {
		conn = new(TConn)
	} else {
		conn = x.(*TConn)
	}
	conn.onGet(id, node, netConn, rawConn)
	return conn
}
func putTConn(conn *TConn) {
	conn.onPut()
	poolTConn.Put(conn)
}

func (c *TConn) onGet(id int64, node *tcpxNode, netConn net.Conn, rawConn syscall.RawConn) {
	c.tcpxConn_.onGet(id, node, netConn, rawConn)

	c.node = node
}
func (c *TConn) onPut() {
	c.node = nil

	c.tcpxConn_.onPut()
}

func (c *TConn) CloseRead() {
	c._checkClose()
}
func (c *TConn) CloseWrite() {
	if c.node.UDSMode() {
		c.netConn.(*net.UnixConn).CloseWrite()
	} else if c.node.TLSMode() {
		c.netConn.(*tls.Conn).CloseWrite()
	} else {
		c.netConn.(*net.TCPConn).CloseWrite()
	}
	c._checkClose()
}
func (c *TConn) _checkClose() {
	if c.closeSema.Add(-1) == 0 {
		c.Close()
	}
}

func (c *TConn) Close() error {
	c.node.DecSubConn()
	netConn := c.netConn
	putTConn(c)
	return netConn.Close()
}
