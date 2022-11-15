// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Unix domain socket client implementation.

package internal

import (
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

func init() {
	registerFixture(signUnix)
	registerBackend("unixBackend", func(name string, stage *Stage) backend {
		b := new(UnixBackend)
		b.init(name, stage)
		return b
	})
}

// unixClient is the interface for UnixOutgate and UnixBackend.
type unixClient interface {
	client
	streamHolder
}

// unixClient_
type unixClient_ struct {
	// Mixins
	client_
	streamHolder_
	// States
}

func (c *unixClient_) init(name string, stage *Stage) {
	c.client_.init(name, stage)
}

func (c *unixClient_) onConfigure() {
	c.client_.onConfigure()
	// maxStreamsPerConn
	c.ConfigureInt32("maxStreamsPerConn", &c.maxStreamsPerConn, func(value int32) bool { return value > 0 }, 1000)
}
func (c *unixClient_) onPrepare() {
	c.client_.onPrepare()
}
func (c *unixClient_) onShutdown() {
	c.client_.onShutdown()
}

const signUnix = "unix"

func createUnix(stage *Stage) *UnixOutgate {
	unix := new(UnixOutgate)
	unix.init(stage)
	unix.setShell(unix)
	return unix
}

// UnixOutgate component.
type UnixOutgate struct {
	// Mixins
	unixClient_
	// States
}

func (f *UnixOutgate) init(stage *Stage) {
	f.unixClient_.init(signUnix, stage)
}

func (f *UnixOutgate) OnConfigure() {
	f.unixClient_.onConfigure()
}
func (f *UnixOutgate) OnPrepare() {
	f.unixClient_.onPrepare()
}
func (f *UnixOutgate) OnShutdown() {
	f.unixClient_.onShutdown()
}

func (f *UnixOutgate) run() { // goroutine
	for !f.IsShut() {
		// TODO
		time.Sleep(time.Second)
	}
	if Debug(2) {
		fmt.Println("unix done")
	}
	f.stage.SubDone()
}

func (f *UnixOutgate) Dial(address string) (*XConn, error) {
	// TODO
	return nil, nil
}
func (f *UnixOutgate) FetchConn(address string) (*XConn, error) {
	// TODO
	return nil, nil
}
func (f *UnixOutgate) StoreConn(conn *XConn) {
	// TODO
}

// UnixBackend component.
type UnixBackend struct {
	// Mixins
	unixClient_
	loadBalancer_
	// States
	healthCheck any         // TODO
	nodes       []*unixNode // nodes of backend
}

func (b *UnixBackend) init(name string, stage *Stage) {
	b.unixClient_.init(name, stage)
	b.loadBalancer_.init()
}

func (b *UnixBackend) OnConfigure() {
	b.unixClient_.onConfigure()
	b.loadBalancer_.onConfigure(b)
}
func (b *UnixBackend) OnPrepare() {
	b.unixClient_.onPrepare()
	b.loadBalancer_.onPrepare(len(b.nodes))
}
func (b *UnixBackend) OnShutdown() {
	b.unixClient_.onShutdown()
	b.loadBalancer_.onShutdown()
}

func (b *UnixBackend) maintain() { // goroutine
	for _, node := range b.nodes {
		b.IncSub(1)
		go node.maintain()
	}
	b.WaitSubs()
	if Debug(2) {
		fmt.Printf("unixBackend=%s done\n", b.Name())
	}
	b.stage.SubDone()
}

func (b *UnixBackend) Dial() (PConn, error) {
	node := b.nodes[b.getIndex()]
	return node.dial()
}
func (b *UnixBackend) FetchConn() (PConn, error) {
	node := b.nodes[b.getIndex()]
	return node.fetchConn()
}
func (b *UnixBackend) StoreConn(conn PConn) {
	xConn := conn.(*XConn)
	xConn.node.storeConn(xConn)
}

// unixNode is a node in UnixBackend.
type unixNode struct {
	// Mixins
	node_
	// Assocs
	backend *UnixBackend
	// States
}

func (n *unixNode) init(id int32, backend *UnixBackend) {
	n.node_.init(id)
	n.backend = backend
}

func (n *unixNode) maintain() { // goroutine
	// TODO: health check
	for !n.backend.IsShut() {
		time.Sleep(time.Second)
	}
	if Debug(2) {
		fmt.Printf("unixNode=%d done\n", n.id)
	}
	n.backend.SubDone()
}

func (n *unixNode) dial() (*XConn, error) { // some protocols don't support or need connection reusing, just dial & close.
	// TODO
	return nil, nil
}
func (n *unixNode) fetchConn() (*XConn, error) {
	// TODO
	return nil, nil
}
func (n *unixNode) storeConn(xConn *XConn) {
	// TODO
}

// poolXConn
var poolXConn sync.Pool

func getXConn(id int64, client unixClient, node *unixNode, unixConn *net.UnixConn) *XConn {
	var conn *XConn
	if x := poolXConn.Get(); x == nil {
		conn = new(XConn)
	} else {
		conn = x.(*XConn)
	}
	conn.onGet(id, client, node, unixConn)
	return conn
}
func putXConn(conn *XConn) {
	conn.onPut()
	poolXConn.Put(conn)
}

// XConn is a client-side connection to unixNode.
type XConn struct { // only exported to hemi
	// Mixins
	pConn_
	// Conn states (non-zeros)
	node     *unixNode     // associated node if client is UnixBackend
	unixConn *net.UnixConn // unix conn
	// Conn states (zeros)
}

func (c *XConn) onGet(id int64, client unixClient, node *unixNode, unixConn *net.UnixConn) {
	c.pConn_.onGet(id, client, client.MaxStreamsPerConn())
	c.node = node
	c.unixConn = unixConn
}
func (c *XConn) onPut() {
	c.pConn_.onPut()
	c.node = nil
	c.unixConn = nil
}

func (c *XConn) getClient() unixClient { return c.client.(unixClient) }

func (c *XConn) SetWriteDeadline(deadline time.Time) error {
	if deadline.Sub(c.lastWrite) >= c.client.WriteTimeout()/4 {
		if err := c.unixConn.SetWriteDeadline(deadline); err != nil {
			return err
		}
		c.lastWrite = deadline
	}
	return nil
}
func (c *XConn) SetReadDeadline(deadline time.Time) error {
	if deadline.Sub(c.lastRead) >= c.client.ReadTimeout()/4 {
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

func (c *XConn) CloseWrite() error { return c.unixConn.CloseWrite() }

func (c *XConn) Close() error { // only used by clients of dial
	unixConn := c.unixConn
	putXConn(c)
	return unixConn.Close()
}

func (c *XConn) closeConn() { c.unixConn.Close() } // used by clients other than dial
