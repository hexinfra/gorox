// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Unit tests.

package hemi

import (
	"bytes"
	"testing"
)

func TestHTTPMethods(t *testing.T) {
	methodCodes := map[string]uint32{
		"GET":     MethodGET,
		"HEAD":    MethodHEAD,
		"POST":    MethodPOST,
		"PUT":     MethodPUT,
		"DELETE":  MethodDELETE,
		"CONNECT": MethodCONNECT,
		"OPTIONS": MethodOPTIONS,
		"TRACE":   MethodTRACE,
	}
	methods := bytes.Split(httpMethodBytes, []byte(" "))
	for _, method := range methods {
		hash := bytesHash(method)
		m := httpMethodTable[httpMethodFind(hash)]
		if m.hash != hash {
			t.Error("invalid hash")
		}
		if !bytes.Equal(httpMethodBytes[m.from:m.edge], method) {
			t.Error("invalid from edge")
		}
		if m.code != methodCodes[string(method)] {
			t.Error("invalid code")
		}
	}
}

func TestSpan(t *testing.T) {
	s := span{3, 4}
	if s.size() != 1 {
		t.Error("t.size()")
	}
}
func BenchmarkStringHash(b *testing.B) {
	s := "hello-world"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stringHash(s)
	}
}
func BenchmarkBytesHash(b *testing.B) {
	p := []byte("hello-world")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bytesHash(p)
	}
}
