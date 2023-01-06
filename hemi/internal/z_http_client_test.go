// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Unit tests.

package internal

import (
	"bytes"
	"testing"
)

func TestMultipleResponseHeaders(t *testing.T) {
	headers := bytes.Split(httpResponseMultipleHeaderBytes, []byte(" "))
	for _, header := range headers {
		hash := bytesHash(header)
		h := httpResponseMultipleHeaderTable[httpResponseMultipleHeaderFind(hash)]
		if h.hash != hash {
			t.Error("hash invalid")
		}
		if !bytes.Equal(httpResponseMultipleHeaderBytes[h.from:h.edge], header) {
			t.Error("from edge invalid")
		}
	}
}
func TestCriticalResponseHeaders(t *testing.T) {
	headers := bytes.Split(httpResponseCriticalHeaderBytes, []byte(" "))
	for _, header := range headers {
		hash := bytesHash(header)
		h := httpResponseCriticalHeaderTable[httpResponseCriticalHeaderFind(hash)]
		if h.hash != hash {
			t.Error("hash invalid")
		}
		if !bytes.Equal(httpResponseCriticalHeaderBytes[h.from:h.edge], header) {
			t.Error("from edge invalid")
		}
	}
}
