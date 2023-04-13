// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// GWEB agent handlet passes requests to backend GWEB servers and cache responses.

package internal

// gwebAgent
type gwebAgent struct {
}

// gStream
type gStream struct {
}

// gRequest
type gRequest struct {
	gwebOut_
}

// gResponse
type gResponse struct {
	gwebIn_
}
