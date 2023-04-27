// Copyright (c) 2020-2023 Feng Wei <feng19910104@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Access tests.
package access

import (
	"errors"
	"testing"
)

func TestParseRule(t *testing.T) {
	tests := []struct {
		input  string
		expect error
	}{
		{"all", nil},
		{"", errors.New("false")},
		{"127.0.0.1", nil},
		{"127.0.0.", errors.New("false")},
		{"127.0.0.*", errors.New("false")},
		{"192.168.6.1/24", nil},
		{"192.168.6.1/16", nil},
		{"192.168.6.1/4", nil},
		{"::1", nil},
		{"fe80::180a:f009:f396:192", nil},
		{"::192.1.56.10/96", nil},
		{"fe80::180a:f009:f396:192/64", nil},
		{"82:6e:6e:e8:3c:", errors.New("false")},
	}

	for idx, test := range tests {
		recv := checkRule([]string{test.input})
		if recv == nil && test.expect != recv {
			t.Errorf("#%d: recv=%v, expect=%v", idx, recv, test.expect)
		}
		if recv != nil && test.expect == nil {
			t.Errorf("#%d: recv=%v, expect=%v", idx, recv, test.expect)
		}
	}
}

func TestHandle(t *testing.T) {

	// TODO: mockRequest
	// TODO: mockResponse
}
