// Copyright (c) 2020-2023 Feng Wei <feng19910104@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Referer tests.

package referer

import (
	"bytes"
	"reflect"
	"regexp"
	"testing"

	. "github.com/hexinfra/gorox/hemi/internal"
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
		{"http://gorox.io?a=1", []byte("gorox.io"), nil, 7},
		{"http://localhost?a=1", []byte("localhost"), nil, 7},
		{"http://localhost:8080/", []byte("localhost"), []byte("/"), 7},
		{"http:///", nil, []byte("/"), 7},
		{"https://", nil, nil, 8},
		{"http://", nil, nil, 7},
		{"gorox.io:8080?a=1", []byte("gorox.io"), nil, 0},
		{"gorox.io?a=1", []byte("gorox.io"), nil, 0},
		{"gorox.io", []byte("gorox.io"), nil, 0},
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
		expect bool
	}{
		{"*.gorox.com", true},
		{"*.gorox.*", false},
		{"gorox.com", true},
		{"www.gorox.*", true},
		{"gorox.*", true},
		{"gorox.*/app", true},
		{"www.gorox.com/app", true},
		{"www.gorox.com:8080/app", true},
		{"http://www.gorox.com:8080/app", false},
		{`~\.gorox\.`, true},
		{`~.*`, true},
		{`~.*/app`, true},
		{`~(?-i)gorox.net`, true},
		{`~]\`, false},
	}

	for idx, test := range tests {
		recv := checkRule([][]byte{[]byte(test.input)})
		if recv != test.expect {
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
			[]byte("*.gorox.com/app"),
			&refererRule{suffixMatch, []byte("*.gorox.com"), []byte("/app"), nil},
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

func TestHandle(t *testing.T) {
	tests := []struct {
		referer     string
		serverNames []byte
		noneReferer bool
		isBlocked   bool
		expect      bool
	}{
		{"http://www.example.org", []byte("www.example.org"), false, false, true},
		{"http://www.example.org", nil, true, false, true},
		{"http://", nil, false, true, true},
		{"", nil, false, true, false},
		{"", nil, true, false, true},
		{"xxxxxx", []byte("*.gorox.org"), false, true, true},
		{"http://localhost", []byte("localhost"), false, false, true},
		{"http://www.gorox.org", []byte("*.gorox.org"), false, false, true},
		{"http://www.gorox.org", []byte("gorox.org"), false, false, false},
		{"http://www.gorox.org", []byte("gorox.*"), false, false, false},
		{"http://gorox.org", []byte("gorox.*"), false, false, true},
		{"http://gorox.com", []byte("gorox.*"), false, false, true},
		{"http://gorox.com/a/p/p", []byte("gorox.*/a/p/p"), false, false, true},
		{"http://gorox.com/a/p/p", []byte("gorox.*/a/"), false, false, true},
		{"http://gorox.com:8080/a/p/p", []byte("gorox.*"), false, false, true},
		{"http://bar.com:8080/a/p/p", []byte(`~bar.org`), false, false, false},
		{"http://h5.bar.com:8080/a/p/p", []byte(`~\.bar\.`), false, false, true},
		{"http://h5.bar.com:8080/a/p/p", []byte(`~(?-i).bar.com`), false, false, true},
		{"http://www.gorox.org/uri", []byte(`~gorox.org/uri`), false, false, true},
		{"http://www.gorox.org", []byte(`~gorox.org/uri`), false, false, false},
		{"http://www.gorox.org/uRI", []byte(`~gorox.org/uri`), false, false, false},
		{"http://www.gorox.org/uRI", []byte(`~example.org$`), false, false, false},
		{"http://book.h5.gorox.org", []byte(`~example.org$`), false, false, false},
	}

	for idx, test := range tests {
		checker := &refererChecker{}
		checker.serverNames = [][]byte{test.serverNames}
		checker.NoneReferer = test.noneReferer
		checker.IsBlocked = test.isBlocked
		checker.OnPrepare()

		httpRaw := "GET /app HTTP/1.1\r\n" +
			"Host: localhost\r\n"
		if len(test.referer) > 0 {
			httpRaw += "Referer: " + test.referer
		}
		httpRaw += "\r\n\r\n"

		req, resp := MockHttp1([]byte(httpRaw))
		recv := checker.Handle(req, resp)
		if recv != test.expect {
			t.Errorf("#%d: recv=%v, expect=%v", idx, recv, test.expect)
		}
		if !recv && resp.Status() != 403 {
			t.Errorf("#%d: recv=%v, expect=403", idx, resp.Status())
		}
	}
}
