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

func TestFCGIResponseMultipleHeaders(t *testing.T) {
	headers := bytes.Split(fcgiResponseMultipleHeaderNames, []byte(" "))
	for _, header := range headers {
		hash := bytesHash(header)
		h := fcgiResponseMultipleHeaderTable[fcgiResponseMultipleHeaderFind(hash)]
		if h.hash != hash {
			t.Error("hash invalid")
		}
		if !bytes.Equal(fcgiResponseMultipleHeaderNames[h.from:h.edge], header) {
			t.Error("from edge invalid")
		}
	}
}
func TestFCGIResponseCriticalHeaders(t *testing.T) {
	headers := bytes.Split(fcgiResponseCriticalHeaderNames, []byte(" "))
	for _, header := range headers {
		hash := bytesHash(header)
		h := fcgiResponseCriticalHeaderTable[fcgiResponseCriticalHeaderFind(hash)]
		if h.hash != hash {
			t.Error("hash invalid")
		}
		if !bytes.Equal(fcgiResponseCriticalHeaderNames[h.from:h.edge], header) {
			t.Error("from edge invalid")
		}
	}
}
