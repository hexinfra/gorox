// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// TCP/TLS client implementation.

package internal

import (
	"crypto/tls"
	"io"
	"net"
	"sync"
	"syscall"
	"time"
)

func init() {
	registerFixture(signTCPSOutgate)
	registerBackend("tcpsBackend", func(name string, stage *Stage) backend {
		b := new(TCPSBackend)
		b.onCreate(name, stage)
		return b
	})
}

// tcpsClient is the interface for TCPSOutgate and TCPSBackend.
type tcpsClient interface {
	client
	streamHolder
}

// tcpsClient_
type tcpsClient_ struct {
	// Mixins
	streamHolder_
	// States
}

func (t *tcpsClient_) onCreate() {
}

func (t *tcpsClient_) onConfigure(shell Component) {
	t.streamHolder_.onConfigure(shell, 1000)
}
func (t *tcpsClient_) onPrepare(shell Component) {
	t.streamHolder_.onPrepare(shell)
}

const signTCPSOutgate = "tcpsOutgate"

func createTCPSOutgate(stage *Stage) *TCPSOutgate {
	tcps := new(TCPSOutgate)
	tcps.onCreate(stage)
	tcps.setShell(tcps)
	return tcps
}

// TCPSOutgate component.
type TCPSOutgate struct {
	// Mixins
	outgate_
	tcpsClient_
	// States
}

func (f *TCPSOutgate) onCreate(stage *Stage) {
	f.outgate_.onCreate(signTCPSOutgate, stage)
	f.tcpsClient_.onCreate()
}

func (f *TCPSOutgate) OnConfigure() {
	f.outgate_.onConfigure()
	f.tcpsClient_.onConfigure(f)
}
func (f *TCPSOutgate) OnPrepare() {
	f.outgate_.onPrepare()
	f.tcpsClient_.onPrepare(f)
}

func (f *TCPSOutgate) run() { // goroutine
	Loop(time.Second, f.Shut, func(now time.Time) {
		// TODO
	})
	if IsDebug(2) {
		Debugln("tcpsOutgate done")
	}
	f.stage.SubDone()
}

func (f *TCPSOutgate) Dial(address string, tlsMode bool) (*TConn, error) {
	netConn, err := net.DialTimeout("tcp", address, f.dialTimeout)
	if err != nil {
		return nil, err
	}
	connID := f.nextConnID()
	if tlsMode {
		tlsConn := tls.Client(netConn, f.tlsConfig)
		return getTConn(connID, f, nil, tlsConn, nil), nil
	} else {
		rawConn, err := netConn.(*net.TCPConn).SyscallConn()
		if err != nil {
			netConn.Close()
			return nil, err
		}
		return getTConn(connID, f, nil, netConn, rawConn), nil
	}
}

// TCPSBackend component.
type TCPSBackend struct {
	// Mixins
	backend_[*tcpsNode]
	tcpsClient_
	loadBalancer_
	// States
	health any // TODO
}

func (b *TCPSBackend) onCreate(name string, stage *Stage) {
	b.backend_.onCreate(name, stage, b)
	b.tcpsClient_.onCreate()
	b.loadBalancer_.init()
}

func (b *TCPSBackend) OnConfigure() {
	b.backend_.onConfigure()
	b.tcpsClient_.onConfigure(b)
	b.loadBalancer_.onConfigure(b)
}
func (b *TCPSBackend) OnPrepare() {
	b.backend_.onPrepare()
	b.tcpsClient_.onPrepare(b)
	b.loadBalancer_.onPrepare(len(b.nodes))
}

func (b *TCPSBackend) createNode(id int32) *tcpsNode {
	node := new(tcpsNode)
	node.init(id, b)
	return node
}

func (b *TCPSBackend) Dial() (PConn, error) {
	node := b.nodes[b.getNext()]
	return node.dial()
}

func (b *TCPSBackend) FetchConn() (PConn, error) {
	node := b.nodes[b.getNext()]
	return node.fetchConn()
}
func (b *TCPSBackend) StoreConn(conn PConn) {
	tConn := conn.(*TConn)
	tConn.node.storeConn(tConn)
}

// tcpsNode is a node in TCPSBackend.
type tcpsNode struct {
	// Mixins
	wireNode_
	// Assocs
	// States
}

func (n *tcpsNode) init(id int32, backend *TCPSBackend) {
	n.wireNode_.init(id, backend)
}

func (n *tcpsNode) maintain(shut chan struct{}) { // goroutine
	Loop(time.Second, shut, func(now time.Time) {
		// TODO: health check
	})
	n.markDown()
	if size := n.closeFree(); size > 0 {
		n.IncSub(0 - size)
	}
	n.WaitSubs() // conns
	if IsDebug(2) {
		Debugf("tcpsNode=%d done\n", n.id)
	}
	n.backend.SubDone()
}

func (n *tcpsNode) dial() (*TConn, error) { // some protocols don't support or need connection reusing, just dial & tConn.close.
	if IsDebug(2) {
		Debugf("tcpsNode=%d dial %s\n", n.id, n.address)
	}
	backend := n.backend.(*TCPSBackend)
	netConn, err := net.DialTimeout("tcp", n.address, backend.dialTimeout)
	if err != nil {
		n.markDown()
		return nil, err
	}
	if IsDebug(2) {
		Debugf("tcpsNode=%d dial %s OK!\n", n.id, n.address)
	}
	connID := backend.nextConnID()
	if backend.tlsMode {
		tlsConn := tls.Client(netConn, backend.tlsConfig)
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
		n.IncSub(1)
	}
	return tConn, err
}
func (n *tcpsNode) storeConn(tConn *TConn) {
	if tConn.IsBroken() || n.isDown() || !tConn.isAlive() {
		if IsDebug(2) {
			Debugf("TConn[node=%d id=%d] closed\n", tConn.node.id, tConn.id)
		}
		n.closeConn(tConn)
	} else {
		if IsDebug(2) {
			Debugf("TConn[node=%d id=%d] pushed\n", tConn.node.id, tConn.id)
		}
		n.pushConn(tConn)
	}
}

func (n *tcpsNode) closeConn(tConn *TConn) {
	tConn.closeConn()
	putTConn(tConn)
	n.SubDone()
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
	node    *tcpsNode       // associated node if client is TCPSBackend
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

func (c *TConn) CloseWrite() error {
	if c.client.TLSMode() {
		return c.netConn.(*tls.Conn).CloseWrite()
	} else {
		return c.netConn.(*net.TCPConn).CloseWrite()
	}
}

func (c *TConn) Close() error { // only used by clients of dial
	netConn := c.netConn
	putTConn(c)
	return netConn.Close()
}

func (c *TConn) closeConn() { c.netConn.Close() } // used by codes other than dial
