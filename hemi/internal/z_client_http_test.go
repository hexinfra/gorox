// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Unit tests.

package internal

import (
	"bytes"
	"testing"
)

func TestHRequestCrucialHeaders(t *testing.T) {
	headers := bytes.Split(hRequestCrucialHeaderNames, []byte(" "))
	if len(headers) != len(hRequestCrucialHeaderTable) {
		t.Error("size mismatch")
	}
	for _, header := range headers {
		hash := bytesHash(header)
		h := hRequestCrucialHeaderTable[hRequestCrucialHeaderFind(hash)]
		if h.hash != hash {
			t.Error("hash invalid")
		}
		if !bytes.Equal(hRequestCrucialHeaderNames[h.from:h.edge], header) {
			t.Error("from edge invalid")
		}
	}
}
func TestHResponseCriticalHeaders(t *testing.T) {
	headers := bytes.Split(hResponseCriticalHeaderNames, []byte(" "))
	if len(headers) != len(hResponseCriticalHeaderTable) {
		t.Error("size mismatch")
	}
	for _, header := range headers {
		hash := bytesHash(header)
		h := hResponseCriticalHeaderTable[hResponseCriticalHeaderFind(hash)]
		if h.hash != hash {
			t.Error("hash invalid")
		}
		if !bytes.Equal(hResponseCriticalHeaderNames[h.from:h.edge], header) {
			t.Error("from edge invalid")
		}
	}
}
func TestHResponseMultipleHeaders(t *testing.T) {
	headers := bytes.Split(hResponseMultipleHeaderNames, []byte(" "))
	if len(headers) != len(hResponseMultipleHeaderTable) {
		t.Error("size mismatch")
	}
	for _, header := range headers {
		hash := bytesHash(header)
		h := hResponseMultipleHeaderTable[hResponseMultipleHeaderFind(hash)]
		if h.hash != hash {
			t.Error("hash invalid")
		}
		if !bytes.Equal(hResponseMultipleHeaderNames[h.from:h.edge], header) {
			t.Error("from edge invalid")
		}
	}
}
