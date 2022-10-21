// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// QUIC stream.

package quix

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
func (s *Stream) Read(p []byte) (n int, err error) {
	return
}
func (s *Stream) Write(p []byte) (n int, err error) {
	return
}
func (s *Stream) Close() error {
	return nil
}
