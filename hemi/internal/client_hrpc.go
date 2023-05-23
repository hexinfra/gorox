// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HRPC client implementation.

package internal

// hrpcBackend
type hrpcBackend struct {
}

// hrpcNode
type hrpcNode struct {
}

// HCall
type HCall struct {
	// Mixins
	clientCall_
	// Assocs
	req  HReq
	resp HResp
}

// HReq
type HReq struct {
}

// HResp
type HResp struct {
}
