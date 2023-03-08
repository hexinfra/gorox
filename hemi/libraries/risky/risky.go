// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Conversion between string and []byte.

package risky

import (
	"unsafe"
)

// Refer
type Refer struct { // same as string, 16 bytes
	Ptr unsafe.Pointer
	Len int
}

func ReferTo(p []byte) Refer {
	return *(*Refer)(unsafe.Pointer(&p))
}
func (r *Refer) Reset() {
	r.Ptr, r.Len = nil, 0
}
func (r Refer) Bytes() (p []byte) {
	h := (*Bytes)(unsafe.Pointer(&p))
	h.Ptr, h.Len, h.Cap = r.Ptr, r.Len, r.Len
	return
}

// Bytes
type Bytes struct { // same as []byte, 24 bytes
	Ptr unsafe.Pointer
	Len int
	Cap int
}

func ConstBytes(s string) (p []byte) { // WARNING: *DO NOT* change s through p!
	return unsafe.Slice(unsafe.StringData(s), len(s))
}
func WeakString(p []byte) (s string) { // WARNING: *DO NOT* change p while using s!
	return unsafe.String(unsafe.SliceData(p), len(p))
}
