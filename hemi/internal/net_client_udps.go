// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// UDPS (UDP/TLS/UDS) client implementation.

package internal

import (
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// udpsClient is the interface for *UDPSOutgate and *UDPSBackend.
type udpsClient interface {
	// Imports
	client
	// Methods
}

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

func getUConn(id int64, sockType int8, tlsMode bool, client udpsClient, node *udpsNode, udpConn *net.UDPConn, rawConn syscall.RawConn) *UConn {
	var conn *UConn
	if x := poolUConn.Get(); x == nil {
		conn = new(UConn)
	} else {
		conn = x.(*UConn)
	}
	conn.onGet(id, sockType, tlsMode, client, node, udpConn, rawConn)
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
	node    *udpsNode       // associated node if client is UDPSBackend
	udpConn *net.UDPConn    // udp conn
	rawConn syscall.RawConn // for syscall
	// Conn states (zeros)
	broken atomic.Bool // is conn broken?
}

func (l *UConn) onGet(id int64, sockType int8, tlsMode bool, client udpsClient, node *udpsNode, udpConn *net.UDPConn, rawConn syscall.RawConn) {
	l.Conn_.onGet(id, sockType, tlsMode, client)
	l.node = node
	l.udpConn = udpConn
	l.rawConn = rawConn
}
func (l *UConn) onPut() {
	l.Conn_.onPut()
	l.node = nil
	l.udpConn = nil
	l.rawConn = nil
	l.broken.Store(false)
}

func (l *UConn) getClient() udpsClient { return l.client.(udpsClient) }

func (l *UConn) SetWriteDeadline(deadline time.Time) error {
	if deadline.Sub(l.lastWrite) >= time.Second {
		if err := l.udpConn.SetWriteDeadline(deadline); err != nil {
			return err
		}
		l.lastWrite = deadline
	}
	return nil
}
func (l *UConn) SetReadDeadline(deadline time.Time) error {
	if deadline.Sub(l.lastRead) >= time.Second {
		if err := l.udpConn.SetReadDeadline(deadline); err != nil {
			return err
		}
		l.lastRead = deadline
	}
	return nil
}

func (l *UConn) Write(p []byte) (n int, err error) { return l.udpConn.Write(p) }
func (l *UConn) Read(p []byte) (n int, err error)  { return l.udpConn.Read(p) }

func (l *UConn) isBroken() bool { return l.broken.Load() }
func (l *UConn) markBroken()    { l.broken.Store(true) }

func (l *UConn) Close() error { // only used by clients of dial
	udpConn := l.udpConn
	putUConn(l)
	return udpConn.Close()
}

func (l *UConn) closeConn() { l.udpConn.Close() } // used by codes which use fetch/store
