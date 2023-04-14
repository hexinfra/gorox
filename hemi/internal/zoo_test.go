// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Unit tests.

package internal

import (
	"bytes"
	"testing"
)

/*
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
*/

/*
func TestWRequestCriticalHeaders(t *testing.T) {
	headers := bytes.Split(wRequestCriticalHeaderNames, []byte(" "))
	if len(headers) != len(wRequestCriticalHeaderTable) {
		t.Error("size mismatch")
	}
	for _, header := range headers {
		hash := bytesHash(header)
		h := wRequestCriticalHeaderTable[wRequestCriticalHeaderFind(hash)]
		if h.hash != hash {
			t.Error("hash invalid")
		}
		if !bytes.Equal(wRequestCriticalHeaderNames[h.from:h.edge], header) {
			t.Error("from edge invalid")
		}
	}
}
func TestWResponseSingletonHeaders(t *testing.T) {
	headers := bytes.Split(wResponseSingletonHeaderNames, []byte(" "))
	if len(headers) != len(wResponseSingletonHeaderTable) {
		t.Error("size mismatch")
	}
	for _, header := range headers {
		hash := bytesHash(header)
		h := wResponseSingletonHeaderTable[wResponseSingletonHeaderFind(hash)]
		if h.hash != hash {
			t.Error("hash invalid")
		}
		if !bytes.Equal(wResponseSingletonHeaderNames[h.from:h.edge], header) {
			t.Error("from edge invalid")
		}
	}
}
func TestWResponseImportantHeaders(t *testing.T) {
	headers := bytes.Split(wResponseImportantHeaderNames, []byte(" "))
	if len(headers) != len(wResponseImportantHeaderTable) {
		t.Error("size mismatch")
	}
	for _, header := range headers {
		hash := bytesHash(header)
		h := wResponseImportantHeaderTable[wResponseImportantHeaderFind(hash)]
		if h.hash != hash {
			t.Error("hash invalid")
		}
		if !bytes.Equal(wResponseImportantHeaderNames[h.from:h.edge], header) {
			t.Error("from edge invalid")
		}
	}
}
*/

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

/*
func TestWebRequestSingletonHeaders(t *testing.T) {
	headers := bytes.Split(webRequestSingletonHeaderNames, []byte(" "))
	if len(headers) != len(webRequestSingletonHeaderTable) {
		t.Error("size mismatch")
	}
	for _, header := range headers {
		hash := bytesHash(header)
		h := webRequestSingletonHeaderTable[webRequestSingletonHeaderFind(hash)]
		if h.hash != hash {
			t.Error("hash invalid")
		}
		if !bytes.Equal(webRequestSingletonHeaderNames[h.from:h.edge], header) {
			t.Error("from edge invalid")
		}
	}
}
func TestWebRequestImportantHeaders(t *testing.T) {
	headers := bytes.Split(webRequestImportantHeaderNames, []byte(" "))
	if len(headers) != len(webRequestImportantHeaderTable) {
		t.Error("size mismatch")
	}
	for _, header := range headers {
		hash := bytesHash(header)
		h := webRequestImportantHeaderTable[webRequestImportantHeaderFind(hash)]
		if h.hash != hash {
			t.Error("hash invalid")
		}
		if !bytes.Equal(webRequestImportantHeaderNames[h.from:h.edge], header) {
			t.Error("from edge invalid")
		}
	}
}
func TestWebResponseCriticalHeaders(t *testing.T) {
	headers := bytes.Split(webResponseCriticalHeaderNames, []byte(" "))
	if len(headers) != len(webResponseCriticalHeaderTable) {
		t.Error("size mismatch")
	}
	for _, header := range headers {
		hash := bytesHash(header)
		h := webResponseCriticalHeaderTable[webResponseCriticalHeaderFind(hash)]
		if h.hash != hash {
			t.Error("hash invalid")
		}
		if !bytes.Equal(webResponseCriticalHeaderNames[h.from:h.edge], header) {
			t.Error("from edge invalid")
		}
	}
}
*/
