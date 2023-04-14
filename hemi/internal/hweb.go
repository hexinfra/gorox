// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HWEB protocol elements.

// HWEB is an HTTP gateway protocol like FCGI, but has a lot of improvements over FCGI.

package internal

// hwebIn_ is used by hwebRequest and hResponse.
type hwebIn_ = webIn_

func (r *hwebIn_) readContentH() (p []byte, err error) {
	return
}

// hwebOut_ is used by hwebResponse and hRequest.
type hwebOut_ = webOut_

func (r *hwebOut_) sendChainH() error {
	return nil
}

//////////////////////////////////////// HWEB protocol elements.
