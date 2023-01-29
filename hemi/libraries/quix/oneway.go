// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// QUIC oneway.

package quix

import (
	"time"
)

// Oneway
type Oneway struct {
	conn *Conn
	id   int64
}

func (o *Oneway) SetReadDeadline(deadline time.Time) error {
	return nil
}
func (o *Oneway) SetWriteDeadline(deadline time.Time) error {
	return nil
}
func (o *Oneway) Read(p []byte) (n int, err error) {
	return
}
func (o *Oneway) Write(p []byte) (n int, err error) {
	return
}
func (o *Oneway) Close() error {
	return nil
}
