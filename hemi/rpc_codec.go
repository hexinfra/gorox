// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// General RPC incoming and outgoing messages implementation.

package hemi

import (
	"errors"
	"time"
)

// rpcAgent collects shared methods between rpcServer or rpcBackend.
type rpcAgent interface {
	// Imports
	// Methods
	Stage() *Stage
	ReadTimeout() time.Duration
	RecvTimeout() time.Duration // timeout to recv the whole message content
	WriteTimeout() time.Duration
	SendTimeout() time.Duration // timeout to send the whole message
}

// _rpcAgent_
type _rpcAgent_ struct {
	// States
	recvTimeout time.Duration // timeout to recv the whole message content
	sendTimeout time.Duration // timeout to send the whole message
}

func (a *_rpcAgent_) onConfigure(shell Component, sendTimeout time.Duration, recvTimeout time.Duration) {
	// sendTimeout
	shell.ConfigureDuration("sendTimeout", &a.sendTimeout, func(value time.Duration) error {
		if value > 0 {
			return nil
		}
		return errors.New(".sendTimeout has an invalid value")
	}, sendTimeout)

	// recvTimeout
	shell.ConfigureDuration("recvTimeout", &a.recvTimeout, func(value time.Duration) error {
		if value > 0 {
			return nil
		}
		return errors.New(".recvTimeout has an invalid value")
	}, recvTimeout)
}
func (a *_rpcAgent_) onPrepare(shell Component) {
}

func (a *_rpcAgent_) RecvTimeout() time.Duration { return a.recvTimeout }
func (a *_rpcAgent_) SendTimeout() time.Duration { return a.sendTimeout }

// rpcConn
type rpcConn interface {
	// TODO
}

// _rpcConn_
type _rpcConn_ struct {
}

// rpcExchan is the interface for *hrpcExchan and *HExchan.
type rpcExchan interface {
	// TODO
}

// _rpcExchan_
type _rpcExchan_ struct {
}

// rpcIn is the interface for *hrpcRequest and *HResponse. Used as shell by rpcIn_.
type rpcIn interface {
	// TODO
}

// rpcIn_ is the parent for rpcServerRequest_ and rpcBackendResponse_.
type rpcIn_ struct {
	// Assocs
	shell rpcIn
	// TODO
	rpcIn0
}
type rpcIn0 struct {
	arrayKind int8 // kind of current r.array. see arrayKindXXX
}

// rpcOut is the interface for *hrpcResponse and *HRequest. Used as shell by rpcOut_.
type rpcOut interface {
	// TODO
}

// rpcOut_ is the parent for rpcServerResponse_ and rpcBackendRequest_.
type rpcOut_ struct {
	// Assocs
	shell rpcOut
	// TODO
	rpcOut0
}
type rpcOut0 struct {
}
