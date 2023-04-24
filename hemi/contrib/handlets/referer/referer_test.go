// Copyright (c) 2020-2023 Feng Wei <feng19910104@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Referer tests.

package referer

import (
	"bytes"
	"testing"
)

func TestGetHostNameAndPath(t *testing.T) {
	tests := []struct {
		input   string
		expect1 []byte
		expect2 []byte
	}{
		{"http://www.a.org/a/b/c", []byte("www.a.org"), []byte("/a/b/c")},
		{"https://www.a.org/a/b/c", []byte("www.a.org"), []byte("/a/b/c")},
		{"https://www.a.org:8080/a/b/c", []byte("www.a.org"), []byte("/a/b/c")},
		{"www.a.org/a/b/c", []byte("www.a.org"), []byte("/a/b/c")},
		{"www.a.org:8080/a?a=1", []byte("www.a.org"), []byte("/a")},
		{"http://www.a.org?a=1", []byte("www.a.org"), nil},
		{"http://a.io?a=1", []byte("a.io"), nil},
		{"http://localhost?a=1", []byte("localhost"), nil},
		{"http://localhost:8080/", []byte("localhost"), []byte("/")},
		{"http:///", nil, []byte("/")},
		{"https://", nil, nil},
		{"http://", nil, nil},
		{"a.io:8080?a=1", []byte("a.io"), nil},
		{"a.io?a=1", []byte("a.io"), nil},
		{"a.io", []byte("a.io"), nil},
		{"localhost:8080", []byte("localhost"), nil},
		{"https///", []byte("https"), []byte("///")},
	}

	for idx, test := range tests {
		recv1, recv2 := getHostNameAndPath([]byte(test.input))
		if !bytes.Equal(recv1, test.expect1) {
			t.Errorf("#%d: recv1=%s, expect1=%s", idx, recv1, test.expect1)
		}

		if !bytes.Equal(recv2, test.expect2) {
			t.Errorf("#%d: recv2=%s, expect2=%s", idx, recv2, test.expect2)
		}
	}
}

func TestHandle(t *testing.T) {

	// TODO: mockRequest
	// TODO: mockResponse
}
