// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// TUDS (TCP over Unix Domain Socket) client implementation.

package internal

import (
	"io"
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

func init() {
	registerFixture(signTUDSOutgate)
	RegisterBackend("tudsBackend", func(name string, stage *Stage) Backend {
		b := new(TUDSBackend)
		b.onCreate(name, stage)
		return b
	})
}

const signTUDSOutgate = "tudsOutgate"

func createTUDSOutgate(stage *Stage) *TUDSOutgate {
	tuds := new(TUDSOutgate)
	tuds.onCreate(stage)
	tuds.setShell(tuds)
	return tuds
}

// TUDSOutgate component.
type TUDSOutgate struct {
	// Mixins
	outgate_
	streamHolder_
	// States
}

func (f *TUDSOutgate) onCreate(stage *Stage) {
	f.outgate_.onCreate(signTUDSOutgate, stage)
}

func (f *TUDSOutgate) OnConfigure() {
	f.outgate_.onConfigure()
	f.streamHolder_.onConfigure(f, 1000)
}
func (f *TUDSOutgate) OnPrepare() {
	f.outgate_.onPrepare()
	f.streamHolder_.onPrepare(f)
}

func (f *TUDSOutgate) run() { // goroutine
	f.Loop(time.Second, func(now time.Time) {
		// TODO
	})
	if IsDebug(2) {
		Println("tudsOutgate done")
	}
	f.stage.SubDone()
}

func (f *TUDSOutgate) Dial(address string) (*XConn, error) {
	unixAddr, err := net.ResolveUnixAddr("unix", address)
	if err != nil {
		return nil, err
	}
	unixConn, err := net.DialUnix("unix", nil, unixAddr)
	if err != nil {
		return nil, err
	}
	connID := f.nextConnID()
	rawConn, err := unixConn.SyscallConn()
	if err != nil {
		unixConn.Close()
		return nil, err
	}
	return getXConn(connID, f, nil, unixConn, rawConn), nil
}

// TUDSBackend component.
type TUDSBackend struct {
	// Mixins
	Backend_[*tudsNode]
	streamHolder_
	loadBalancer_
	// States
	health any // TODO
}

func (b *TUDSBackend) onCreate(name string, stage *Stage) {
	b.Backend_.onCreate(name, stage, b)
	b.loadBalancer_.init()
}

func (b *TUDSBackend) OnConfigure() {
	b.Backend_.onConfigure()
	b.streamHolder_.onConfigure(b, 1000)
	b.loadBalancer_.onConfigure(b)
}
func (b *TUDSBackend) OnPrepare() {
	b.Backend_.onPrepare()
	b.streamHolder_.onPrepare(b)
	b.loadBalancer_.onPrepare(len(b.nodes))
}

func (b *TUDSBackend) createNode(id int32) *tudsNode {
	node := new(tudsNode)
	node.init(id, b)
	return node
}

func (b *TUDSBackend) Dial() (wireConn, error) { return b.DialTUDS() }
func (b *TUDSBackend) DialTUDS() (*XConn, error) {
	node := b.nodes[b.getNext()]
	return node.dial()
}

func (b *TUDSBackend) FetchConn() (wireConn, error) { return b.FetchXConn() }
func (b *TUDSBackend) FetchXConn() (*XConn, error) {
	node := b.nodes[b.getNext()]
	return node.fetchConn()
}

func (b *TUDSBackend) StoreConn(conn wireConn) { b.StoreXConn(conn.(*XConn)) }
func (b *TUDSBackend) StoreXConn(xConn *XConn) {
	xConn.node.storeConn(xConn)
}

// tudsNode is a node in TUDSBackend.
type tudsNode struct {
	// Mixins
	Node_
	// Assocs
	backend *TUDSBackend
	// States
	unixAddr *net.UnixAddr
}

func (n *tudsNode) init(id int32, backend *TUDSBackend) {
	n.Node_.init(id)
	n.backend = backend
}

func (n *tudsNode) setAddress(address string) {
	unixAddr, err := net.ResolveUnixAddr("unix", address)
	if err != nil {
		UseExitln(err.Error())
		return
	}
	n.Node_.setAddress(address)
	n.unixAddr = unixAddr
}

func (n *tudsNode) Maintain() { // goroutine
	n.Loop(time.Second, func(now time.Time) {
		// TODO: health check, markUp()
	})
	n.markDown()
	if size := n.closeFree(); size > 0 {
		n.IncSub(0 - size)
	}
	n.WaitSubs() // conns
	if IsDebug(2) {
		Printf("tudsNode=%d done\n", n.id)
	}
	n.backend.SubDone()
}

func (n *tudsNode) dial() (*XConn, error) { // some protocols don't support or need connection reusing, just dial & xConn.close.
	if IsDebug(2) {
		Printf("tudsNode=%d dial %s\n", n.id, n.address)
	}
	unixConn, err := net.DialUnix("unix", nil, n.unixAddr)
	if err != nil {
		n.markDown()
		return nil, err
	}
	if IsDebug(2) {
		Printf("tudsNode=%d dial %s OK!\n", n.id, n.address)
	}
	connID := n.backend.nextConnID()
	rawConn, err := unixConn.SyscallConn()
	if err != nil {
		unixConn.Close()
		return nil, err
	}
	return getXConn(connID, n.backend, n, unixConn, rawConn), nil
}

func (n *tudsNode) fetchConn() (*XConn, error) {
	conn := n.pullConn()
	down := n.isDown()
	if conn != nil {
		xConn := conn.(*XConn)
		if xConn.isAlive() && !xConn.reachLimit() && !down {
			return xConn, nil
		}
		n.closeConn(xConn)
	}
	if down {
		return nil, errNodeDown
	}
	xConn, err := n.dial()
	if err == nil {
		n.IncSub(1)
	}
	return xConn, err
}
func (n *tudsNode) storeConn(xConn *XConn) {
	if xConn.IsBroken() || n.isDown() || !xConn.isAlive() {
		if IsDebug(2) {
			Printf("XConn[node=%d id=%d] closed\n", xConn.node.id, xConn.id)
		}
		n.closeConn(xConn)
	} else {
		if IsDebug(2) {
			Printf("XConn[node=%d id=%d] pushed\n", xConn.node.id, xConn.id)
		}
		n.pushConn(xConn)
	}
}

func (n *tudsNode) closeConn(xConn *XConn) {
	xConn.closeConn()
	putXConn(xConn)
	n.SubDone()
}

// poolXConn
var poolXConn sync.Pool

func getXConn(id int64, client tClient, node *tudsNode, unixConn *net.UnixConn, rawConn syscall.RawConn) *XConn {
	var conn *XConn
	if x := poolXConn.Get(); x == nil {
		conn = new(XConn)
	} else {
		conn = x.(*XConn)
	}
	conn.onGet(id, client, node, unixConn, rawConn)
	return conn
}
func putXConn(conn *XConn) {
	conn.onPut()
	poolXConn.Put(conn)
}

// XConn is a client-side connection to tudsNode.
type XConn struct {
	// Mixins
	Conn_
	// Conn states (non-zeros)
	node       *tudsNode       // associated node if client is TUDSBackend
	unixConn   *net.UnixConn   // unix conn
	rawConn    syscall.RawConn // for syscall
	maxStreams int32           // how many streams are allowed on this conn?
	// Conn states (zeros)
	counter     atomic.Int64 // used to make temp name
	usedStreams atomic.Int32 // how many streams has been used?
	writeBroken atomic.Bool  // write-side broken?
	readBroken  atomic.Bool  // read-side broken?
}

func (c *XConn) onGet(id int64, client tClient, node *tudsNode, unixConn *net.UnixConn, rawConn syscall.RawConn) {
	c.Conn_.onGet(id, client)
	c.node = node
	c.unixConn = unixConn
	c.rawConn = rawConn
	c.maxStreams = client.MaxStreamsPerConn()
}
func (c *XConn) onPut() {
	c.Conn_.onPut()
	c.node = nil
	c.unixConn = nil
	c.rawConn = nil
	c.counter.Store(0)
	c.usedStreams.Store(0)
	c.writeBroken.Store(false)
	c.readBroken.Store(false)
}

func (c *XConn) getClient() tClient { return c.client.(tClient) }

func (c *XConn) UnixConn() *net.UnixConn { return c.unixConn }

func (c *XConn) reachLimit() bool { return c.usedStreams.Add(1) > c.maxStreams }

func (c *XConn) MakeTempName(p []byte, unixTime int64) (from int, edge int) {
	return makeTempName(p, int64(c.client.Stage().ID()), c.id, unixTime, c.counter.Add(1))
}

func (c *XConn) SetWriteDeadline(deadline time.Time) error {
	if deadline.Sub(c.lastWrite) >= time.Second {
		if err := c.unixConn.SetWriteDeadline(deadline); err != nil {
			return err
		}
		c.lastWrite = deadline
	}
	return nil
}
func (c *XConn) SetReadDeadline(deadline time.Time) error {
	if deadline.Sub(c.lastRead) >= time.Second {
		if err := c.unixConn.SetReadDeadline(deadline); err != nil {
			return err
		}
		c.lastRead = deadline
	}
	return nil
}

func (c *XConn) Write(p []byte) (n int, err error)         { return c.unixConn.Write(p) }
func (c *XConn) Writev(vector *net.Buffers) (int64, error) { return vector.WriteTo(c.unixConn) }
func (c *XConn) Read(p []byte) (n int, err error)          { return c.unixConn.Read(p) }
func (c *XConn) ReadFull(p []byte) (n int, err error)      { return io.ReadFull(c.unixConn, p) }
func (c *XConn) ReadAtLeast(p []byte, min int) (n int, err error) {
	return io.ReadAtLeast(c.unixConn, p, min)
}

func (c *XConn) IsBroken() bool { return c.writeBroken.Load() || c.readBroken.Load() }
func (c *XConn) MarkBroken() {
	c.markWriteBroken()
	c.markReadBroken()
}

func (c *XConn) markWriteBroken() { c.writeBroken.Store(true) }
func (c *XConn) markReadBroken()  { c.readBroken.Store(true) }

func (c *XConn) CloseWrite() error { return c.unixConn.CloseWrite() }

func (c *XConn) Close() error { // only used by clients of dial
	unixConn := c.unixConn
	putXConn(c)
	return unixConn.Close()
}

func (c *XConn) closeConn() { c.unixConn.Close() } // used by codes which use fetch/store
