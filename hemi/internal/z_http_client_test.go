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
	headers := bytes.Split(httpMultipleResponseHeaderBytes, []byte(" "))
	for _, header := range headers {
		hash := bytesHash(header)
		h := httpMultipleResponseHeaderTable[httpMultipleResponseHeaderFind(hash)]
		if h.hash != hash {
			t.Error("hash invalid")
		}
		if !bytes.Equal(httpMultipleResponseHeaderBytes[h.from:h.edge], header) {
			t.Error("from edge invalid")
		}
	}
}
func TestCriticalResponseHeaders(t *testing.T) {
	headers := bytes.Split(httpCriticalResponseHeaderBytes, []byte(" "))
	for _, header := range headers {
		hash := bytesHash(header)
		h := httpCriticalResponseHeaderTable[httpCriticalResponseHeaderFind(hash)]
		if h.hash != hash {
			t.Error("hash invalid")
		}
		if !bytes.Equal(httpCriticalResponseHeaderBytes[h.from:h.edge], header) {
			t.Error("from edge invalid")
		}
	}
}
