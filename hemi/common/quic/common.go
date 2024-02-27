// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Common elements.

package quic

import (
	"sync"
)

var poolDatagram sync.Pool

func getDatagram() []byte {
	if x := poolDatagram.Get(); x == nil {
		return make([]byte, 1200)
	} else {
		return x.([]byte)
	}
}
func putDatagram(d []byte) {
	if len(d) != 1200 {
		panic("putDatagram mismatch")
	}
	poolDatagram.Put(d)
}
