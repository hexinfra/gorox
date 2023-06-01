// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// QUDS (QUIC over UUDS) client implementation.

package internal

import (
	"time"
)

// qudsClient is the interface for QUDSOutgate and QUDSBackend.
type qudsClient interface {
	client
	streamHolder
}

// QUDSOutgate component.
type QUDSOutgate struct {
	// Mixins
	outgate_
	streamHolder_
	// States
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

func (b *QUDSBackend) OnConfigure() {
	// TODO
}
func (b *QUDSBackend) OnPrepare() {
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
