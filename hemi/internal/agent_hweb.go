// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HWEB agent handlet passes requests to backend HWEB servers and cache responses.

package internal

// hwebAgent
type hwebAgent struct {
}

// bConn
type bConn struct {
}

// bStream
type bStream struct {
}

// bRequest
type bRequest struct {
	httpOut_
}

// bResponse
type bResponse struct {
	httpIn_
}
