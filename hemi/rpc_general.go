// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// General RPC elements.

package hemi

import (
	"time"
)

// rpcBroker
type rpcBroker interface {
	// TODO
}

// rpcBroker_
type rpcBroker_ struct {
	// TODO
}

func (b *rpcBroker_) onConfigure(shell Component, sendTimeout time.Duration, recvTimeout time.Duration) {
}
func (b *rpcBroker_) onPrepare(shell Component) {
}

// rpcConn
type rpcConn interface {
	// TODO
}

// rpcExchan is the interface for *hrpcExchan and *HExchan.
type rpcExchan interface {
	// TODO
}
