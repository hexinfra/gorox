// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Conversion between string and []byte.

package risky

import (
	"reflect"
	"unsafe"
)

func init() {
	// ensure compatability
	if unsafe.Sizeof(Refer{}) != unsafe.Sizeof(reflect.StringHeader{}) {
		panic("layout of string has changed!")
	}
	if unsafe.Sizeof(Bytes{}) != unsafe.Sizeof(reflect.SliceHeader{}) {
		panic("layout of []byte has changed!")
	}
}

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
	sh := (*Refer)(unsafe.Pointer(&s))
	ph := (*Bytes)(unsafe.Pointer(&p))
	ph.Ptr, ph.Len, ph.Cap = sh.Ptr, sh.Len, sh.Len
	return
}
func WeakString(p []byte) (s string) { // WARNING: *DO NOT* change p while using s!
	return *(*string)(unsafe.Pointer(&p))
}
