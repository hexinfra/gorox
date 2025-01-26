// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Unit tests for the whole Hemi Engine.

package hemi

import (
	"bytes"
	"testing"
)

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
