// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// QUIC stream.

package quic

import (
	"time"
)

// Stream
type Stream struct {
	conn *Conn
	id   int64
}

func (s *Stream) SetReadDeadline(deadline time.Time) error {
	return nil
}
func (s *Stream) SetWriteDeadline(deadline time.Time) error {
	return nil
}
func (s *Stream) Read(dst []byte) (n int, err error) {
	return
}
func (s *Stream) Write(src []byte) (n int, err error) {
	return
}
func (s *Stream) Close() error {
	return nil
}
