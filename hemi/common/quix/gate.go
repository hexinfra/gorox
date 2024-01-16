// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// QUIC gate.

package quix

// Gate
type Gate struct {
	address string
}

func NewGate(address string) *Gate {
	g := new(Gate)
	g.address = address
	return g
}

func (g *Gate) Open() error {
	// reuseport by default
	return nil
}
func (g *Gate) Accept() (*Connection, error) {
	return nil, nil
}
func (g *Gate) Close() error {
	return nil
}
