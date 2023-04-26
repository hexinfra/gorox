// Copyright (c) 2020-2023 Feng Weei <feng19910104@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Allows you to easily mock up various services.
package internal

import (
	"io"
	"net"
	"time"
)

type mockNetIO struct {
	rb []byte
	wb []byte
}

func (m *mockNetIO) Read(b []byte) (int, error) {
	n := copy(b[:], m.rb)
	m.rb = m.rb[n:]
	if m.rb == nil {
		return n, io.EOF
	}
	return n, nil
}
func (m *mockNetIO) Write(b []byte) (int, error) {
	m.wb = append(m.wb, b...)
	return len(b), nil
}
func (m *mockNetIO) Close() error                       { return nil }
func (m *mockNetIO) SetDeadline(t time.Time) error      { return nil }
func (m *mockNetIO) SetReadDeadline(t time.Time) error  { return nil }
func (m *mockNetIO) SetWriteDeadline(t time.Time) error { return nil }
func (m *mockNetIO) LocalAddr() net.Addr                { return nil }
func (m *mockNetIO) RemoteAddr() net.Addr               { return nil }

// TODO: temporary function, which will be refactored later.
// rb is the http raw data.
func MockHttp1(rb []byte) (Request, Response) {
	SetDebug(2)
	s := new(httpxServer)
	s.onCreate("", createStage())
	gate := new(httpxGate)
	gate.init(s, 1)

	mockNetIO := &mockNetIO{rb: rb, wb: make([]byte, 0, _4K)}
	httpConn := getHTTP1Conn(1, s, gate, mockNetIO, nil)
	conn := httpConn.(*http1Conn)

	conn.stream.onUse(conn)
	req, resp := &conn.stream.request, &conn.stream.response
	req.recvHead()

	return req, resp
}
