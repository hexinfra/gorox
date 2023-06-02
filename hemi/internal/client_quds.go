// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// QUDS (QUIC over UUDS) client implementation.

package internal

import (
	"time"
)

func init() {
	registerFixture(signQUDSOutgate)
	RegisterBackend("qudsBackend", func(name string, stage *Stage) Backend {
		b := new(QUDSBackend)
		b.onCreate(name, stage)
		return b
	})
}

const signQUDSOutgate = "qudsOutgate"

func createQUDSOutgate(stage *Stage) *QUDSOutgate {
	quds := new(QUDSOutgate)
	quds.onCreate(stage)
	quds.setShell(quds)
	return quds
}

// QUDSOutgate component.
type QUDSOutgate struct {
	// Mixins
	outgate_
	streamHolder_
	// States
}

func (f *QUDSOutgate) onCreate(stage *Stage) {
	f.outgate_.onCreate(signQUDSOutgate, stage)
}

func (f *QUDSOutgate) OnConfigure() {
	f.outgate_.onConfigure()
	f.streamHolder_.onConfigure(f, 1000)
}
func (f *QUDSOutgate) OnPrepare() {
	f.outgate_.onPrepare()
	f.streamHolder_.onPrepare(f)
}

func (f *QUDSOutgate) run() { // goroutine
	f.Loop(time.Second, func(now time.Time) {
		// TODO
	})
	if IsDebug(2) {
		Println("qudsOutgate done")
	}
	f.stage.SubDone()
}

func (f *QUDSOutgate) Dial(address string) (*DConn, error) {
	// TODO
	return nil, nil
}
func (f *QUDSOutgate) FetchConn(address string) {
	// TODO
}
func (f *QUDSOutgate) StoreConn(dConn *DConn) {
	// TODO
}

// QUDSBackend component.
type QUDSBackend struct {
	// Mixins
	Backend_[*qudsNode]
	streamHolder_
	loadBalancer_
	// States
	health any // TODO
}

func (b *QUDSBackend) onCreate(name string, stage *Stage) {
	b.Backend_.onCreate(name, stage, b)
	b.loadBalancer_.init()
}

func (b *QUDSBackend) OnConfigure() {
	b.Backend_.onConfigure()
	b.streamHolder_.onConfigure(b, 1000)
	b.loadBalancer_.onConfigure(b)
}
func (b *QUDSBackend) OnPrepare() {
	b.Backend_.onPrepare()
	b.streamHolder_.onPrepare(b)
	b.loadBalancer_.onPrepare(len(b.nodes))
}

func (b *QUDSBackend) createNode(id int32) *qudsNode {
	node := new(qudsNode)
	node.init(id, b)
	return node
}

func (b *QUDSBackend) Dial() (*DConn, error) {
	// TODO
	return nil, nil
}
func (b *QUDSBackend) FetchConn() (*DConn, error) {
	// TODO
	return nil, nil
}
func (b *QUDSBackend) StoreConn(dConn *DConn) {
	// TODO
}

// qudsNode is a node in QUDSBackend.
type qudsNode struct {
	// Mixins
	Node_
	// Assocs
	backend *QUDSBackend
	// States
}

func (n *qudsNode) init(id int32, backend *QUDSBackend) {
	n.Node_.init(id)
	n.backend = backend
}

func (n *qudsNode) Maintain() { // goroutine
	n.Loop(time.Second, func(now time.Time) {
		// TODO: health check
	})
	// TODO: wait for all conns
	if IsDebug(2) {
		Printf("qudsNode=%d done\n", n.id)
	}
	n.backend.SubDone()
}

func (n *qudsNode) dial() (*DConn, error) {
	// TODO
	return nil, nil
}
func (n *qudsNode) fetchConn() (*DConn, error) {
	// Note: A DConn can be used concurrently, limited by maxStreams.
	// TODO
	return nil, nil
}
func (n *qudsNode) storeConn(dConn *DConn) {
	// Note: A DConn can be used concurrently, limited by maxStreams.
	// TODO
}

// DConn is a client-side connection to qudsNode.
type DConn struct {
	// Mixins
	conn_
	// Conn states (non-zeros)
	// Conn states (zeros)
}

// DStream is a bidirectional stream of DConn.
type DStream struct {
	// TODO
	conn *DConn
}

// DOneway is a unidirectional stream of DConn.
type DOneway struct {
	// TODO
	conn *DConn
}
