// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// General RPC backend implementation.

package internal

import (
	"time"
)

// rpcBackend_
type rpcBackend_[N Node] struct {
	// Mixins
	Backend_[N]
	rpcBroker_
	// States
	health any // TODO
}

func (b *rpcBackend_[N]) onCreate(name string, stage *Stage, creator interface{ createNode(id int32) N }) {
	b.Backend_.onCreate(name, stage, creator)
}

func (b *rpcBackend_[N]) onConfigure(shell Component) {
	b.Backend_.onConfigure()
}
func (b *rpcBackend_[N]) onPrepare(shell Component, numNodes int) {
	b.Backend_.onPrepare()
}

// backendWire_
type backendWire_ struct {
	Conn_
	rpcWire_
	backend rpcBackend
}

func (w *backendWire_) onGet(id int64, udsMode bool, tlsMode bool, backend rpcBackend) {
	w.Conn_.onGet(id, udsMode, tlsMode, time.Now().Add(backend.AliveTimeout()))
	w.rpcWire_.onGet()
	w.backend = backend
}
func (w *backendWire_) onPut() {
	w.backend = nil
	w.Conn_.onPut()
	w.rpcWire_.onPut()
}

func (w *backendWire_) rpcBackend() rpcBackend { return w.backend }

// backendCall_
type backendCall_ struct {
}

func (c *backendCall_) onGet(id int64, udsMode bool, tlsMode bool, backend rpcBackend) {
	//c.Conn_.onGet(id, udsMode, tlsMode, time.Now().Add(backend.AliveTimeout()))
	// c.rpcCall_.onGet()?
	//c.backend = backend
}
func (c *backendCall_) onPut() {
	//c.backend = nil
	//c.Conn_.onPut()
	// c.rpcCall_.onPut()?
}

// backendReq_
type backendReq_ struct {
	// Mixins
	rpcOut_
}

// backendResp_
type backendResp_ struct {
	// Mixins
	rpcIn_
}
