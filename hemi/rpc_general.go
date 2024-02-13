// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// General RPC elements.

package hemi

import (
	"errors"
	"time"
)

// rpcAgent
type rpcAgent interface {
	// Imports
	agent
	// Methods
	RecvTimeout() time.Duration // timeout to recv the whole message content
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

// rpcExchan is the interface for *hrpcExchan and *HExchan.
type rpcExchan interface {
	// TODO
}
