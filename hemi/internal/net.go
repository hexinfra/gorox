// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Network router and related components.

package internal

// _router is the interface for *QUICRouter, *TCPSRouter, *UDPSRouter.
type _router interface {
	Component
	serve() // runner
}

// _gate is the interface for *quicGate, *tcpsGate, *udpsGate.
type _gate interface {
	open() error
	shut() error
}

// _dealet is for QUICDealet, TCPSDealet, UDPSDealet.
type _dealet interface {
	Component
}

// _case is the interface for *quicCase, *tcpsCase, *udpsCase.
type _case interface {
	Component
}

// Buffer
type Buffer struct {
	// TODO
}
