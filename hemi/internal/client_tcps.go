// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// TCP/TLS client implementation.

package internal

import (
	"crypto/tls"
	"fmt"
	"github.com/hexinfra/gorox/hemi/libraries/system"
	"io"
	"net"
	"sync"
	"syscall"
	"time"
)

func init() {
	registerFixture(signTCPS)
	registerBackend("tcpsBackend", func(name string, stage *Stage) backend {
		b := new(TCPSBackend)
		b.init(name, stage)
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
	client_
	streamHolder_
	// States
}

func (c *tcpsClient_) init(name string, stage *Stage) {
	c.client_.init(name, stage)
}

func (c *tcpsClient_) onConfigure() {
	c.client_.onConfigure()
	// maxStreamsPerConn
	c.ConfigureInt32("maxStreamsPerConn", &c.maxStreamsPerConn, func(value int32) bool { return value > 0 }, 1000)
}
func (c *tcpsClient_) onPrepare() {
	c.client_.onPrepare()
}
func (c *tcpsClient_) onShutdown() {
	c.client_.onShutdown()
}

const signTCPS = "tcps"

func createTCPS(stage *Stage) *TCPSOutgate {
	tcps := new(TCPSOutgate)
	tcps.init(stage)
	tcps.setShell(tcps)
	return tcps
}

// TCPSOutgate component.
type TCPSOutgate struct {
	// Mixins
	tcpsClient_
	// States
}

func (f *TCPSOutgate) init(stage *Stage) {
	f.tcpsClient_.init(signTCPS, stage)
}

func (f *TCPSOutgate) OnConfigure() {
	f.tcpsClient_.onConfigure()
}
func (f *TCPSOutgate) OnPrepare() {
	f.tcpsClient_.onPrepare()
}
func (f *TCPSOutgate) OnShutdown() {
	f.tcpsClient_.onShutdown()
}

func (f *TCPSOutgate) run() { // goroutine
	Loop(time.Second, f.Shut, func(now time.Time) {
		// TODO
	})
	if Debug(2) {
		fmt.Println("tcps done")
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
		tlsConn := tls.Client(netConn, nil)
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
func (f *TCPSOutgate) FetchConn(address string, tlsMode bool) (*TConn, error) {
	// TODO
	return nil, nil
}
func (f *TCPSOutgate) StoreConn(conn *TConn) {
	// TODO
}

// TCPSBackend component.
type TCPSBackend struct {
	// Mixins
	tcpsClient_
	loadBalancer_
	// States
	health any         // TODO
	nodes  []*tcpsNode // nodes of backend
}

func (b *TCPSBackend) init(name string, stage *Stage) {
	b.tcpsClient_.init(name, stage)
	b.loadBalancer_.init()
}

func (b *TCPSBackend) OnConfigure() {
	b.tcpsClient_.onConfigure()
	b.loadBalancer_.onConfigure(b)
	// nodes
	v, ok := b.Find("nodes")
	if !ok {
		UseExitln("nodes is required for backends")
	}
	vNodes, ok := v.List()
	if !ok {
		UseExitln("bad nodes")
	}
	for id, elem := range vNodes {
		vNode, ok := elem.Dict()
		if !ok {
			UseExitln("node in nodes must be a dict")
		}
		node := new(tcpsNode)
		node.init(int32(id), b)
		// address
		vAddress, ok := vNode["address"]
		if !ok {
			UseExitln("address is required in node")
		}
		if address, ok := vAddress.String(); ok && address != "" {
			node.address = address
		}
		// weight
		vWeight, ok := vNode["weight"]
		if ok {
			if weight, ok := vWeight.Int32(); ok && weight > 0 {
				node.weight = weight
			} else {
				UseExitln("bad weight in node")
			}
		} else {
			node.weight = 1
		}
		// keepConns
		vKeepConns, ok := vNode["keepConns"]
		if ok {
			if keepConns, ok := vKeepConns.Int32(); ok && keepConns > 0 {
				node.keepConns = keepConns
			} else {
				UseExitln("bad keepConns in node")
			}
		} else {
			node.keepConns = 10
		}
		b.nodes = append(b.nodes, node)
	}
}
func (b *TCPSBackend) OnPrepare() {
	b.tcpsClient_.onPrepare()
	b.loadBalancer_.onPrepare(len(b.nodes))
}
func (b *TCPSBackend) OnShutdown() {
	b.tcpsClient_.onShutdown()
	b.loadBalancer_.onShutdown()
}

func (b *TCPSBackend) maintain() { // goroutine
	shutdown := make(chan struct{})
	for _, node := range b.nodes {
		b.IncSub(1)
		go node.maintain(shutdown)
	}
	<-b.Shut
	close(shutdown)
	b.WaitSubs()
	if Debug(2) {
		fmt.Printf("tcpsBackend=%s done\n", b.Name())
	}
	b.stage.SubDone()
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

func (n *tcpsNode) maintain(shutdown chan struct{}) { // goroutine
	Loop(time.Second, shutdown, func(now time.Time) {
		// TODO: health check
	})
	if Debug(2) {
		fmt.Printf("tcpsNode=%d done\n", n.id)
	}
	n.backend.SubDone()
}

func (n *tcpsNode) dial() (*TConn, error) { // some protocols don't support or need connection reusing, just dial & tConn.close.
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

func (c *TConn) closeConn() { c.netConn.Close() } // used by clients other than dial
