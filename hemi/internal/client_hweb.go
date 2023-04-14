// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HWEB client implementation.

package internal

import (
	"sync"
	"time"
)

// hwebBackend
type hwebBackend struct {
	// Mixins
	webBackend_[*hwebNode]
}

// hwebNode
type hwebNode struct {
	// Mixins
	webNode_
	// Assocs
	backend *hwebBackend
	// States
}

func (n *hwebNode) maintain(shut chan struct{}) { // goroutine
	Loop(time.Second, shut, func(now time.Time) {
		// TODO: health check
	})
	// TODO: wait for all conns
	if IsDebug(2) {
		Debugf("http2Node=%d done\n", n.id)
	}
	n.backend.SubDone()
}

// hConn
type hConn struct {
	wConn_
}

// poolHStream
var poolHStream sync.Pool

func getHStream(conn *hConn, id uint32) *hStream {
	return nil
}
func putHStream(stream *hStream) {
}

// hStream
type hStream struct {
	wStream_
}

// hRequest
type hRequest struct {
	wRequest_
}

// hResponse
type hResponse struct {
	wResponse_
}
