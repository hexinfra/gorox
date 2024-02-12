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

// rpcBroker
type rpcBroker interface {
	// Imports
	broker
	// Methods
	RecvTimeout() time.Duration // timeout to recv the whole message content
	SendTimeout() time.Duration // timeout to send the whole message
}

// rpcBroker_
type rpcBroker_ struct {
	// States
	recvTimeout    time.Duration // timeout to recv the whole message content
	sendTimeout    time.Duration // timeout to send the whole message
}

func (b *rpcBroker_) onConfigure(shell Component, sendTimeout time.Duration, recvTimeout time.Duration) {
	// sendTimeout
	shell.ConfigureDuration("sendTimeout", &b.sendTimeout, func(value time.Duration) error {
		if value > 0 {
			return nil
		}
		return errors.New(".sendTimeout has an invalid value")
	}, sendTimeout)

	// recvTimeout
	shell.ConfigureDuration("recvTimeout", &b.recvTimeout, func(value time.Duration) error {
		if value > 0 {
			return nil
		}
		return errors.New(".recvTimeout has an invalid value")
	}, recvTimeout)
}
func (b *rpcBroker_) onPrepare(shell Component) {
}

func (b *rpcBroker_) RecvTimeout() time.Duration { return b.recvTimeout }
func (b *rpcBroker_) SendTimeout() time.Duration { return b.sendTimeout }

// rpcConn
type rpcConn interface {
	// TODO
}

// rpcExchan is the interface for *hrpcExchan and *HExchan.
type rpcExchan interface {
	// TODO
}
