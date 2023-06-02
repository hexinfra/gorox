// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HRPC client implementation.

package internal

import (
	"time"
)

func init() {
	registerFixture(signHRPCOutgate)
}

const signHRPCOutgate = "hrpcOutgate"

func createHRPCOutgate(stage *Stage) *HRPCOutgate {
	hrpc := new(HRPCOutgate)
	hrpc.onCreate(stage)
	hrpc.setShell(hrpc)
	return hrpc
}

// HRPCOutgate
type HRPCOutgate struct {
	// Mixins
	rpcOutgate_
	// States
}

func (f *HRPCOutgate) onCreate(stage *Stage) {
	f.rpcOutgate_.onCreate(signHRPCOutgate, stage)
}

func (f *HRPCOutgate) OnConfigure() {
	f.rpcOutgate_.onConfigure(f)
}
func (f *HRPCOutgate) OnPrepare() {
	f.rpcOutgate_.onPrepare(f)
}

func (f *HRPCOutgate) run() { // goroutine
	f.Loop(time.Second, func(now time.Time) {
		// TODO
	})
	if IsDebug(2) {
		Println("hrpcOutgate done")
	}
	f.stage.SubDone()
}

// HRPCBackend
type HRPCBackend struct {
}

// hrpcNode
type hrpcNode struct {
}

// HCall is the client-side HRPC call.
type HCall struct {
	// Mixins
	clientCall_
	// Assocs
	req  HReq
	resp HResp
	// Call states (stocks)
	// Call states (controlled)
	// Call states (non-zeros)
	node *hrpcNode
	id   int32
	// Call states (zeros)
}

// HReq is the client-side HRPC request.
type HReq struct {
	// Mixins
	clientReq_
	// Call states (stocks)
	// Call states (controlled)
	// Call states (non-zeros)
	// Call states (zeros)
}

// HResp is the client-side HRPC response.
type HResp struct {
	// Mixins
	clientResp_
	// Call states (stocks)
	// Call states (controlled)
	// Call states (non-zeros)
	// Call states (zeros)
}