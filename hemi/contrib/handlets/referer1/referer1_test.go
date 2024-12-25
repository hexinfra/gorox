// Copyright (c) 2020-2024 Feng Wei <feng19910104@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Referer tests.

package referer1

import (
	"bytes"
	"errors"
	"reflect"
	"regexp"
	"testing"
)

func TestGetHostNameAndPath(t *testing.T) {
	tests := []struct {
		input   string
		expect1 []byte
		expect2 []byte
		expect3 int
	}{
		{"http://www.gorox.org/a/b/c", []byte("www.gorox.org"), []byte("/a/b/c"), 7},
		{"https://www.gorox.org/a/b/c", []byte("www.gorox.org"), []byte("/a/b/c"), 8},
		{"https://www.gorox.org:8080/a/b/c", []byte("www.gorox.org"), []byte("/a/b/c"), 8},
		{"www.gorox.org/a/b/c", []byte("www.gorox.org"), []byte("/a/b/c"), 0},
		{"www.gorox.org:8080/a?a=1", []byte("www.gorox.org"), []byte("/a"), 0},
		{"http://www.gorox.org?a=1", []byte("www.gorox.org"), nil, 7},
		{"http://gorox.org?a=1", []byte("gorox.org"), nil, 7},
		{"http://localhost?a=1", []byte("localhost"), nil, 7},
		{"http://localhost:8080/", []byte("localhost"), []byte("/"), 7},
		{"http:///", nil, []byte("/"), 7},
		{"https://", nil, nil, 8},
		{"http://", nil, nil, 7},
		{"gorox.org:8080?a=1", []byte("gorox.org"), nil, 0},
		{"gorox.org?a=1", []byte("gorox.org"), nil, 0},
		{"gorox.org", []byte("gorox.org"), nil, 0},
		{"localhost:8080", []byte("localhost"), nil, 0},
		{"https///", []byte("https"), []byte("///"), 0},
	}

	for idx, test := range tests {
		recv1, recv2, recv3 := getHostNameAndPath([]byte(test.input))
		if !bytes.Equal(recv1, test.expect1) {
			t.Errorf("#%d: recv1=%s, expect1=%s", idx, recv1, test.expect1)
		}

		if !bytes.Equal(recv2, test.expect2) {
			t.Errorf("#%d: recv2=%s, expect2=%s", idx, recv2, test.expect2)
		}

		if recv3 != test.expect3 {
			t.Errorf("#%d: recv3=%v, expect3=%v", idx, recv3, test.expect3)
		}
	}
}

func TestCheckRule(t *testing.T) {
	tests := []struct {
		input  string
		expect error
	}{
		{"*.gorox.com", nil},
		{"*.gorox.*", errors.New("false")},
		{"gorox.com", nil},
		{"www.gorox.*", nil},
		{"gorox.*", nil},
		{"gorox.*/api", nil},
		{"www.gorox.com/api", nil},
		{"www.gorox.com:8080/api", nil},
		{"http://www.gorox.com:8080/api", errors.New("false")},
		{`~\.gorox\.`, nil},
		{`~.*`, nil},
		{`~.*/api`, nil},
		{`~(?-i)gorox.dev`, nil},
		{`~]\`, errors.New("false")},
	}

	for idx, test := range tests {
		recv := checkRule([][]byte{[]byte(test.input)})
		if test.expect == nil && recv != nil {
			t.Errorf("#%d: recv=%v, expect=%v", idx, recv, test.expect)
		} else if test.expect != nil && recv == nil {
			t.Errorf("#%d: recv=%v, expect=%v", idx, recv, test.expect)
		}
	}
}

func TestOnPrepare(t *testing.T) {
	tests := []struct {
		input    []byte
		expected *refererRule
	}{
		{
			[]byte("*.gorox.com"),
			&refererRule{suffixMatch, []byte("*.gorox.com"), nil, nil},
		},
		{
			[]byte("*.gorox.com/api"),
			&refererRule{suffixMatch, []byte("*.gorox.com"), []byte("/api"), nil},
		},

		{
			[]byte("gorox.*"),
			&refererRule{prefixMatch, []byte("gorox.*"), nil, nil}},
		{
			[]byte("www.gorox.com"),
			&refererRule{fullMatch, []byte("www.gorox.com"), nil, nil},
		},
		{
			[]byte(`~\.gorox\.`),
			&refererRule{regexpMatch, nil, nil, regexp.MustCompile(`\.gorox\.`)},
		},
		{
			[]byte(`~.*`),
			&refererRule{regexpMatch, nil, nil, regexp.MustCompile(`.*`)},
		},
	}

	for idx, test := range tests {
		r := &refererChecker{}
		r.serverNames = [][]byte{test.input}
		r.OnPrepare()
		if test.expected == nil && len(r.serverNameRules) == 0 {
			continue
		}

		if !reflect.DeepEqual(test.expected, r.serverNameRules[0]) {
			t.Errorf("#%d: recv=%v, expect=%v", idx, r.serverNameRules[0], test.expected)
		}
	}
}
