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

func TestHTTPRequestMultipleHeaders(t *testing.T) {
	headers := bytes.Split(httpRequestMultipleHeaderNames, []byte(" "))
	for _, header := range headers {
		hash := bytesHash(header)
		h := httpRequestMultipleHeaderTable[httpRequestMultipleHeaderFind(hash)]
		if h.hash != hash {
			t.Error("hash invalid")
		}
		if !bytes.Equal(httpRequestMultipleHeaderNames[h.from:h.edge], header) {
			t.Error("from edge invalid")
		}
	}
}
func TestHTTPRequestCriticalHeaders(t *testing.T) {
	headers := bytes.Split(httpRequestCriticalHeaderNames, []byte(" "))
	for _, header := range headers {
		hash := bytesHash(header)
		h := httpRequestCriticalHeaderTable[httpRequestCriticalHeaderFind(hash)]
		if h.hash != hash {
			t.Error("hash invalid")
		}
		if !bytes.Equal(httpRequestCriticalHeaderNames[h.from:h.edge], header) {
			t.Error("from edge invalid")
		}
	}
}
func TestHTTPResponseCrucialHeaders(t *testing.T) {
	headers := bytes.Split(httpResponseCrucialHeaderNames, []byte(" "))
	for _, header := range headers {
		hash := bytesHash(header)
		h := httpResponseCrucialHeaderTable[httpResponseCrucialHeaderFind(hash)]
		if h.hash != hash {
			t.Error("hash invalid")
		}
		if !bytes.Equal(httpResponseCrucialHeaderNames[h.from:h.edge], header) {
			t.Error("from edge invalid")
		}
	}
}
