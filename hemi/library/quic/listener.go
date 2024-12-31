// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// QUIC listener.

package quic

// Listener
type Listener struct {
	address string
}

func NewListener(address string) *Listener {
	l := new(Listener)
	l.address = address
	return l
}

func (l *Listener) Open() error {
	// reuseport by default
	return nil
}
func (l *Listener) Accept() (*Conn, error) {
	return nil, nil
}
func (l *Listener) Close() error {
	return nil
}
