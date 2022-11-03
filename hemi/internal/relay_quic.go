// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// QUIC relay filter implementation.

package internal

func init() {
	RegisterQUICFilter("quicRelay", func(name string, stage *Stage, router *QUICRouter) QUICFilter {
		f := new(quicRelay)
		f.init(name, stage, router)
		return f
	})
}

// quicRelay relays QUIC connections to another QUIC server.
type quicRelay struct {
	// Mixins
	QUICFilter_
	proxy_
	// Assocs
	router *QUICRouter
	// States
}

func (f *quicRelay) init(name string, stage *Stage, router *QUICRouter) {
	f.SetName(name)
	f.proxy_.init(stage)
	f.router = router
}

func (f *quicRelay) OnConfigure() {
	f.proxy_.onConfigure(f)
}
func (f *quicRelay) OnPrepare() {
	f.proxy_.onPrepare(f)
}
func (f *quicRelay) OnShutdown() {
	f.proxy_.onShutdown(f)
}

func (f *quicRelay) Process(conn *QUICConn, stream *QUICStream) (next bool) {
	return
}
