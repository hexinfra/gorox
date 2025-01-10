// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Common elements.

package tcp2

import (
	"sync"
)

var poolDatagram sync.Pool

const datagramSize = 1200

func getDatagram() []byte {
	if x := poolDatagram.Get(); x == nil {
		return make([]byte, datagramSize)
	} else {
		return x.([]byte)
	}
}
func putDatagram(d []byte) {
	if len(d) != datagramSize {
		panic("putDatagram mismatch")
	}
	poolDatagram.Put(d)
}
