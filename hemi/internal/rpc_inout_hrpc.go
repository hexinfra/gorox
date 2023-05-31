// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HRPC incoming message and outgoing message implementation.

package internal

// hrpcIn_ is used by hrpcReq and HResp.
type hrpcIn_ = rpcIn_

func (r *hrpcIn_) readContentH() {
	// TODO
}

// hrpcOut_ is used by hrpcResp and HReq.
type hrpcOut_ = rpcOut_

func (r *hrpcOut_) writeBytesH() {
	// TODO
}
