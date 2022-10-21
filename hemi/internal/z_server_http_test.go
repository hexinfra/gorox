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

func TestMultipleRequestHeaders(t *testing.T) {
	headers := bytes.Split(httpMultipleRequestHeaderBytes, []byte(" "))
	for _, header := range headers {
		hash := bytesHash(header)
		h := httpMultipleRequestHeaderTable[httpMultipleRequestHeaderFind(hash)]
		if h.hash != hash {
			t.Error("hash invalid")
		}
		if !bytes.Equal(httpMultipleRequestHeaderBytes[h.from:h.edge], header) {
			t.Error("from edge invalid")
		}
	}
}
func TestCriticalRequestHeaders(t *testing.T) {
	headers := bytes.Split(httpCriticalRequestHeaderBytes, []byte(" "))
	for _, header := range headers {
		hash := bytesHash(header)
		h := httpCriticalRequestHeaderTable[httpCriticalRequestHeaderFind(hash)]
		if h.hash != hash {
			t.Error("hash invalid")
		}
		if !bytes.Equal(httpCriticalRequestHeaderBytes[h.from:h.edge], header) {
			t.Error("from edge invalid")
		}
	}
}
