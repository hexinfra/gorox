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

func TestFCGIResponseSingletonHeaders(t *testing.T) {
	headers := bytes.Split(fcgiResponseSingletonHeaderNames, []byte(" "))
	if len(headers) != len(fcgiResponseSingletonHeaderTable) {
		t.Error("size mismatch")
	}
	for _, header := range headers {
		hash := bytesHash(header)
		h := fcgiResponseSingletonHeaderTable[fcgiResponseSingletonHeaderFind(hash)]
		if h.hash != hash {
			t.Error("hash invalid")
		}
		if !bytes.Equal(fcgiResponseSingletonHeaderNames[h.from:h.edge], header) {
			t.Error("from edge invalid")
		}
	}
}
func TestFCGIResponseImportantHeaders(t *testing.T) {
	headers := bytes.Split(fcgiResponseImportantHeaderNames, []byte(" "))
	if len(headers) != len(fcgiResponseImportantHeaderTable) {
		t.Error("size mismatch")
	}
	for _, header := range headers {
		hash := bytesHash(header)
		h := fcgiResponseImportantHeaderTable[fcgiResponseImportantHeaderFind(hash)]
		if h.hash != hash {
			t.Error("hash invalid")
		}
		if !bytes.Equal(fcgiResponseImportantHeaderNames[h.from:h.edge], header) {
			t.Error("from edge invalid")
		}
	}
}

func TestHRequestCriticalHeaders(t *testing.T) {
	headers := bytes.Split(hRequestCriticalHeaderNames, []byte(" "))
	if len(headers) != len(hRequestCriticalHeaderTable) {
		t.Error("size mismatch")
	}
	for _, header := range headers {
		hash := bytesHash(header)
		h := hRequestCriticalHeaderTable[hRequestCriticalHeaderFind(hash)]
		if h.hash != hash {
			t.Error("hash invalid")
		}
		if !bytes.Equal(hRequestCriticalHeaderNames[h.from:h.edge], header) {
			t.Error("from edge invalid")
		}
	}
}
func TestHResponseSingletonHeaders(t *testing.T) {
	headers := bytes.Split(hResponseSingletonHeaderNames, []byte(" "))
	if len(headers) != len(hResponseSingletonHeaderTable) {
		t.Error("size mismatch")
	}
	for _, header := range headers {
		hash := bytesHash(header)
		h := hResponseSingletonHeaderTable[hResponseSingletonHeaderFind(hash)]
		if h.hash != hash {
			t.Error("hash invalid")
		}
		if !bytes.Equal(hResponseSingletonHeaderNames[h.from:h.edge], header) {
			t.Error("from edge invalid")
		}
	}
}
func TestHResponseImportantHeaders(t *testing.T) {
	headers := bytes.Split(hResponseImportantHeaderNames, []byte(" "))
	if len(headers) != len(hResponseImportantHeaderTable) {
		t.Error("size mismatch")
	}
	for _, header := range headers {
		hash := bytesHash(header)
		h := hResponseImportantHeaderTable[hResponseImportantHeaderFind(hash)]
		if h.hash != hash {
			t.Error("hash invalid")
		}
		if !bytes.Equal(hResponseImportantHeaderNames[h.from:h.edge], header) {
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

func TestHTTPRequestSingletonHeaders(t *testing.T) {
	headers := bytes.Split(httpRequestSingletonHeaderNames, []byte(" "))
	if len(headers) != len(httpRequestSingletonHeaderTable) {
		t.Error("size mismatch")
	}
	for _, header := range headers {
		hash := bytesHash(header)
		h := httpRequestSingletonHeaderTable[httpRequestSingletonHeaderFind(hash)]
		if h.hash != hash {
			t.Error("hash invalid")
		}
		if !bytes.Equal(httpRequestSingletonHeaderNames[h.from:h.edge], header) {
			t.Error("from edge invalid")
		}
	}
}
func TestHTTPRequestImportantHeaders(t *testing.T) {
	headers := bytes.Split(httpRequestImportantHeaderNames, []byte(" "))
	if len(headers) != len(httpRequestImportantHeaderTable) {
		t.Error("size mismatch")
	}
	for _, header := range headers {
		hash := bytesHash(header)
		h := httpRequestImportantHeaderTable[httpRequestImportantHeaderFind(hash)]
		if h.hash != hash {
			t.Error("hash invalid")
		}
		if !bytes.Equal(httpRequestImportantHeaderNames[h.from:h.edge], header) {
			t.Error("from edge invalid")
		}
	}
}
func TestHTTPResponseCriticalHeaders(t *testing.T) {
	headers := bytes.Split(httpResponseCriticalHeaderNames, []byte(" "))
	if len(headers) != len(httpResponseCriticalHeaderTable) {
		t.Error("size mismatch")
	}
	for _, header := range headers {
		hash := bytesHash(header)
		h := httpResponseCriticalHeaderTable[httpResponseCriticalHeaderFind(hash)]
		if h.hash != hash {
			t.Error("hash invalid")
		}
		if !bytes.Equal(httpResponseCriticalHeaderNames[h.from:h.edge], header) {
			t.Error("from edge invalid")
		}
	}
}
