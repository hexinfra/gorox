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

func TestFCGIResponseCriticalHeaders(t *testing.T) {
	headers := bytes.Split(fcgiResponseCriticalHeaderNames, []byte(" "))
	if len(headers) != len(fcgiResponseCriticalHeaderTable) {
		t.Error("size mismatch")
	}
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
func TestFCGIResponseMultipleHeaders(t *testing.T) {
	headers := bytes.Split(fcgiResponseMultipleHeaderNames, []byte(" "))
	if len(headers) != len(fcgiResponseMultipleHeaderTable) {
		t.Error("size mismatch")
	}
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

func TestText(t *testing.T) {
	s := text{3, 4}
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

func TestHTTPRequestCriticalHeaders(t *testing.T) {
	headers := bytes.Split(httpRequestCriticalHeaderNames, []byte(" "))
	if len(headers) != len(httpRequestCriticalHeaderTable) {
		t.Error("size mismatch")
	}
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
func TestHTTPRequestMultipleHeaders(t *testing.T) {
	headers := bytes.Split(httpRequestMultipleHeaderNames, []byte(" "))
	if len(headers) != len(httpRequestMultipleHeaderTable) {
		t.Error("size mismatch")
	}
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
func TestHTTPResponseCrucialHeaders(t *testing.T) {
	headers := bytes.Split(httpResponseCrucialHeaderNames, []byte(" "))
	if len(headers) != len(httpResponseCrucialHeaderTable) {
		t.Error("size mismatch")
	}
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
