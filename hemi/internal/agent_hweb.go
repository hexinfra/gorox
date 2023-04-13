// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HWEB agent handlet passes requests to backend HWEB servers and cache responses.

package internal

// hwebAgent
type hwebAgent struct {
}

// gStream
type gStream struct {
}

// gRequest
type gRequest struct {
	hwebOut_
}

// gResponse
type gResponse struct {
	hwebIn_
}
