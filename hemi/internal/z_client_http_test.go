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

func TestHResponseMultipleHeaders(t *testing.T) {
	headers := bytes.Split(hResponseMultipleHeaderNames, []byte(" "))
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
func TestHResponseCriticalHeaders(t *testing.T) {
	headers := bytes.Split(hResponseCriticalHeaderNames, []byte(" "))
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
