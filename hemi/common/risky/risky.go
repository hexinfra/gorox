// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Conversion between string and []byte.

package risky

import (
	"unsafe"
)

func ConstBytes(s string) (p []byte) { // WARNING: *DO NOT* mutate s through p!
	return unsafe.Slice(unsafe.StringData(s), len(s))
}
func WeakString(p []byte) (s string) { // WARNING: *DO NOT* mutate p while s is in use!
	return unsafe.String(unsafe.SliceData(p), len(p))
}
