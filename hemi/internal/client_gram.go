// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Datagram UDS client implementation.

package internal

import (
	"net"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

func init() {
	RegisterBackend("gramBackend", func(name string, stage *Stage) Backend {
		b := new(GRAMBackend)
		b.onCreate(name, stage)
		return b
	})
}

// GRAMBackend component.
type GRAMBackend struct {
	// Mixins
	Backend_[*gramNode]
	loadBalancer_
	// States
	health any // TODO
}

func (b *GRAMBackend) onCreate(name string, stage *Stage) {
	b.Backend_.onCreate(name, stage, b)
	b.loadBalancer_.init()
}

func (b *GRAMBackend) OnConfigure() {
	b.Backend_.onConfigure()
	b.loadBalancer_.onConfigure(b)
}
func (b *GRAMBackend) OnPrepare() {
	b.Backend_.onPrepare()
	b.loadBalancer_.onPrepare(len(b.nodes))
}

func (b *GRAMBackend) createNode(id int32) *gramNode {
	node := new(gramNode)
	node.init(id, b)
	return node
}

func (b *GRAMBackend) Dial() (*XLink, error) {
	node := b.nodes[b.getNext()]
	return node.dial()
}
func (b *GRAMBackend) FetchLink() (*XLink, error) {
	node := b.nodes[b.getNext()]
	return node.fetchLink()
}
func (b *GRAMBackend) StoreLink(xLink *XLink) {
	xLink.node.storeLink(xLink)
}

// gramNode is a node in GRAMBackend.
type gramNode struct {
	// Mixins
	Node_
	// Assocs
	backend *GRAMBackend
	// States
}

func (n *gramNode) init(id int32, backend *GRAMBackend) {
	n.Node_.init(id)
	n.backend = backend
}

func (n *gramNode) Maintain() { // goroutine
	n.Loop(time.Second, func(now time.Time) {
		// TODO: health check
	})
	// TODO: wait for all links
	if IsDebug(2) {
		Printf("gramNode=%d done\n", n.id)
	}
	n.backend.SubDone()
}

func (n *gramNode) dial() (*XLink, error) {
	// TODO
	return nil, nil
}
func (n *gramNode) fetchLink() (*XLink, error) {
	link := n.pullConn()
	if link != nil {
		xLink := link.(*XLink)
		if xLink.isAlive() {
			return xLink, nil
		}
		xLink.closeConn()
		putXLink(xLink)
	}
	return n.dial()
}
func (n *gramNode) storeLink(xLink *XLink) {
	if xLink.isBroken() || n.isDown() || !xLink.isAlive() {
		xLink.closeConn()
		putXLink(xLink)
	} else {
		n.pushConn(xLink)
	}
}

// poolXLink
var poolXLink sync.Pool

func getXLink(id int64, backend *GRAMBackend, node *gramNode, unixConn *net.UnixConn, rawConn syscall.RawConn) *XLink {
	var link *XLink
	if x := poolXLink.Get(); x == nil {
		link = new(XLink)
	} else {
		link = x.(*XLink)
	}
	link.onGet(id, backend, node, unixConn, rawConn)
	return link
}
func putXLink(link *XLink) {
	link.onPut()
	poolXLink.Put(link)
}

// XLink is a client-side link to gramNode.
type XLink struct { // only exported to hemi
	// Mixins
	conn_
	// Link states (non-zeros)
	node     *gramNode       // associated node if client is GRAMBackend
	unixConn *net.UnixConn   // unix conn
	rawConn  syscall.RawConn // for syscall
	// Link states (zeros)
	broken atomic.Bool // is link broken?
}

func (l *XLink) onGet(id int64, backend *GRAMBackend, node *gramNode, unixConn *net.UnixConn, rawConn syscall.RawConn) {
	l.conn_.onGet(id, backend)
	l.node = node
	l.unixConn = unixConn
	l.rawConn = rawConn
}
func (l *XLink) onPut() {
	l.conn_.onPut()
	l.node = nil
	l.unixConn = nil
	l.rawConn = nil
	l.broken.Store(false)
}

func (l *XLink) getBackend() *GRAMBackend { return l.client.(*GRAMBackend) }

func (l *XLink) SetWriteDeadline(deadline time.Time) error {
	if deadline.Sub(l.lastWrite) >= time.Second {
		if err := l.unixConn.SetWriteDeadline(deadline); err != nil {
			return err
		}
		l.lastWrite = deadline
	}
	return nil
}
func (l *XLink) SetReadDeadline(deadline time.Time) error {
	if deadline.Sub(l.lastRead) >= time.Second {
		if err := l.unixConn.SetReadDeadline(deadline); err != nil {
			return err
		}
		l.lastRead = deadline
	}
	return nil
}

func (l *XLink) Write(p []byte) (n int, err error) { return l.unixConn.Write(p) }
func (l *XLink) Read(p []byte) (n int, err error)  { return l.unixConn.Read(p) }

func (l *XLink) isBroken() bool { return l.broken.Load() }
func (l *XLink) markBroken()    { l.broken.Store(true) }

func (l *XLink) Close() error { // only used by clients of dial
	unixConn := l.unixConn
	putXLink(l)
	return unixConn.Close()
}

func (l *XLink) closeConn() { l.unixConn.Close() } // used by codes other than dial
