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
		"PATCH":   MethodPATCH,
		"LINK":    MethodLINK,
		"UNLINK":  MethodUNLINK,
		"QUERY":   MethodQUERY,
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
