package access

import (
	"testing"
)

func TestParseRule(t *testing.T) {
	tests := []struct {
		input  string
		expect bool
	}{
		{"all", true},
		{"", false},
		{"127.0.0.1", true},
		{"127.0.0.", false},
		{"127.0.0.*", false},
		{"192.168.6.1/24", true},
		{"192.168.6.1/16", true},
		{"192.168.6.1/4", true},
		{"::1", true},
		{"fe80::180a:f009:f396:192", true},
		{"::192.1.56.10/96", true},
		{"fe80::180a:f009:f396:192/64", true},
		{"82:6e:6e:e8:3c:", false},
	}

	for idx, test := range tests {
		recv := checkRule([]string{test.input})
		if recv != test.expect {
			t.Errorf("#%d: recv=%v, expect=%v", idx, recv, test.expect)
		}
	}
}

func TestHandle(t *testing.T) {

	// TODO: mockRequest
	// TODO: mockResponse
}
