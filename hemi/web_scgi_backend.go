// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// SCGI backend implementation. See: https://python.ca/scgi/protocol.txt

package hemi

import (
	"io"
	"net"
	"sync"
	"syscall"
	"time"
)

func init() {
	RegisterBackend("scgiBackend", func(compName string, stage *Stage) Backend {
		b := new(scgiBackend)
		b.onCreate(compName, stage)
		return b
	})
}

// scgiBackend
type scgiBackend struct {
	// Parent
	Backend_[*scgiNode]
	// States
}

func (b *scgiBackend) onCreate(compName string, stage *Stage) {
	b.Backend_.OnCreate(compName, stage)
}

func (b *scgiBackend) OnConfigure() {
	b.Backend_.OnConfigure()

	b.ConfigureNodes()
}
func (b *scgiBackend) OnPrepare() {
	b.Backend_.OnPrepare()

	b.PrepareNodes()
}

func (b *scgiBackend) CreateNode(compName string) Node {
	node := new(scgiNode)
	node.onCreate(compName, b.stage, b)
	b.AddNode(node)
	return node
}

// scgiNode
type scgiNode struct {
	// Parent
	Node_[*scgiBackend]
	// Mixins
	_contentSaver_ // so scgi responses can save their large contents in local file system.
	// States
}

func (n *scgiNode) onCreate(compName string, stage *Stage, backend *scgiBackend) {
	n.Node_.OnCreate(compName, stage, backend)
}

func (n *scgiNode) OnConfigure() {
	n.Node_.OnConfigure()
	n._contentSaver_.onConfigure(n, 0*time.Second, 0*time.Second, TmpDir()+"/web/backends/"+n.backend.compName+"/"+n.compName)
}
func (n *scgiNode) OnPrepare() {
	n.Node_.OnPrepare()
	n._contentSaver_.onPrepare(n, 0755)
}

func (n *scgiNode) Maintain() { // runner
	n.LoopRun(time.Second, func(now time.Time) {
		// TODO: health check, markDown, markUp()
	})
	n.markDown()
	if DebugLevel() >= 2 {
		Printf("scgiNode=%s done\n", n.compName)
	}
	n.backend.DecSub() // node
}

func (n *scgiNode) dial() (*scgiConn, error) {
	if DebugLevel() >= 2 {
		Printf("scgiNode=%s dial %s\n", n.compName, n.address)
	}
	var (
		conn *scgiConn
		err  error
	)
	if n.UDSMode() {
		conn, err = n._dialUDS()
	} else {
		conn, err = n._dialTCP()
	}
	if err != nil {
		return nil, errNodeDown
	}
	n.IncSubConns()
	return conn, err
}
func (n *scgiNode) _dialUDS() (*scgiConn, error) {
	// TODO: dynamic address names?
	netConn, err := net.DialTimeout("unix", n.address, n.DialTimeout())
	if err != nil {
		n.markDown()
		return nil, err
	}
	if DebugLevel() >= 2 {
		Printf("scgiNode=%s dial %s OK!\n", n.compName, n.address)
	}
	connID := n.nextConnID()
	rawConn, err := netConn.(*net.UnixConn).SyscallConn()
	if err != nil {
		netConn.Close()
		return nil, err
	}
	return getSCGIConn(connID, n, netConn, rawConn), nil
}
func (n *scgiNode) _dialTCP() (*scgiConn, error) {
	// TODO: dynamic address names?
	netConn, err := net.DialTimeout("tcp", n.address, n.DialTimeout())
	if err != nil {
		// TODO: handle ephemeral port exhaustion
		n.markDown()
		return nil, err
	}
	if DebugLevel() >= 2 {
		Printf("scgiNode=%s dial %s OK!\n", n.compName, n.address)
	}
	connID := n.nextConnID()
	rawConn, err := netConn.(*net.TCPConn).SyscallConn()
	if err != nil {
		netConn.Close()
		return nil, err
	}
	return getSCGIConn(connID, n, netConn, rawConn), nil
}

// scgiConn
type scgiConn struct {
	// Assocs
	response scgiResponse // the scgi response
	request  scgiRequest  // the scgi request
	// Conn states (stocks)
	stockBuffer [256]byte // a (fake) buffer to workaround Go's conservative escape analysis. must be >= 256 bytes so names can be placed into
	// Conn states (controlled)
	// Conn states (non-zeros)
	id      int64     // the conn id
	node    *scgiNode // the node to which the connection belongs
	region  Region    // a region-based memory pool
	netConn net.Conn
	rawConn syscall.RawConn
	// Conn states (zeros)
	lastWrite time.Time // deadline of last write operation
	lastRead  time.Time // deadline of last read operation
}

var poolSCGIConn sync.Pool

func getSCGIConn(id int64, node *scgiNode, netConn net.Conn, rawConn syscall.RawConn) *scgiConn {
	var conn *scgiConn
	if x := poolSCGIConn.Get(); x == nil {
		conn = new(scgiConn)
		resp, req := &conn.response, &conn.request
		resp.conn = conn
		req.conn = conn
		req.response = resp
	} else {
		conn = x.(*scgiConn)
	}
	conn.onUse(id, node, netConn, rawConn)
	return conn
}
func putSCGIConn(conn *scgiConn) {
	conn.onEnd()
	poolSCGIConn.Put(conn)
}

func (c *scgiConn) onUse(id int64, node *scgiNode, netConn net.Conn, rawConn syscall.RawConn) {
	c.id = id
	c.node = node
	c.region.Init()
	c.netConn = netConn
	c.rawConn = rawConn
	c.response.onUse()
	c.request.onUse()
}
func (c *scgiConn) onEnd() {
	c.request.onEnd()
	c.response.onEnd()
	c.node = nil
	c.region.Free()
	c.netConn = nil
	c.rawConn = nil
}

func (c *scgiConn) MakeTempName(dst []byte, unixTime int64) int {
	return makeTempName(dst, c.node.Stage().ID(), c.id, unixTime, 0)
}

func (c *scgiConn) setReadDeadline() error {
	if deadline := time.Now().Add(c.node.readTimeout); deadline.Sub(c.lastRead) >= time.Second {
		if err := c.netConn.SetReadDeadline(deadline); err != nil {
			return err
		}
		c.lastRead = deadline
	}
	return nil
}
func (c *scgiConn) setWriteDeadline() error {
	if deadline := time.Now().Add(c.node.writeTimeout); deadline.Sub(c.lastWrite) >= time.Second {
		if err := c.netConn.SetWriteDeadline(deadline); err != nil {
			return err
		}
		c.lastWrite = deadline
	}
	return nil
}

func (c *scgiConn) read(dst []byte) (int, error) { return c.netConn.Read(dst) }
func (c *scgiConn) readAtLeast(dst []byte, min int) (int, error) {
	return io.ReadAtLeast(c.netConn, dst, min)
}
func (c *scgiConn) write(src []byte) (int, error)             { return c.netConn.Write(src) }
func (c *scgiConn) writev(srcVec *net.Buffers) (int64, error) { return srcVec.WriteTo(c.netConn) }

func (c *scgiConn) buffer256() []byte          { return c.stockBuffer[:] }
func (c *scgiConn) unsafeMake(size int) []byte { return c.region.Make(size) }

func (c *scgiConn) Close() error {
	netConn := c.netConn
	putSCGIConn(c)
	return netConn.Close()
}

// scgiResponse must implements the backendResponse interface.
type scgiResponse struct { // incoming. needs parsing
	// Assocs
	conn *scgiConn
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	headResult int16 // result of receiving response head. values are same as http status for convenience
	bodyResult int16 // result of receiving response body. values are same as http status for convenience
	// Conn states (zeros)
	_scgiResponse0 // all values in this struct must be zero by default!
}
type _scgiResponse0 struct { // for fast reset, entirely
}

func (r *scgiResponse) onUse() {
	// TODO
}
func (r *scgiResponse) onEnd() {
	// TODO
	r._scgiResponse0 = _scgiResponse0{}
}

func (r *scgiResponse) reuse() {
	r.onEnd()
	r.onUse()
}

func (r *scgiResponse) KeepAlive() int8 { return -1 } // same as "no connection header". TODO: confirm this

func (r *scgiResponse) HeadResult() int16 { return r.headResult }
func (r *scgiResponse) BodyResult() int16 { return r.bodyResult }

// scgiRequest
type scgiRequest struct { // outgoing. needs building
	// Assocs
	conn     *scgiConn
	response *scgiResponse
	// Conn states (stocks)
	// Conn states (controlled)
	// Conn states (non-zeros)
	// Conn states (zeros)
	_scgiRequest0 // all values in this struct must be zero by default!
}
type _scgiRequest0 struct { // for fast reset, entirely
}

func (r *scgiRequest) onUse() {
	// TODO
}
func (r *scgiRequest) onEnd() {
	// TODO
	r._scgiRequest0 = _scgiRequest0{}
}
