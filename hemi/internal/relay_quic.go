// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// QUIC relay runner implementation.

package internal

func init() {
	RegisterQUICRunner("quicRelay", func(name string, stage *Stage, router *QUICRouter) QUICRunner {
		r := new(quicRelay)
		r.init(name, stage, router)
		return r
	})
}

// quicRelay relays QUIC connections to another QUIC server.
type quicRelay struct {
	// Mixins
	QUICRunner_
	proxy_
	// Assocs
	router *QUICRouter
	// States
}

func (r *quicRelay) init(name string, stage *Stage, router *QUICRouter) {
	r.SetName(name)
	r.proxy_.init(stage)
	r.router = router
}

func (r *quicRelay) OnConfigure() {
	r.proxy_.onConfigure(r)
}
func (r *quicRelay) OnPrepare() {
	r.proxy_.onPrepare(r)
}
func (r *quicRelay) OnShutdown() {
	r.proxy_.onShutdown(r)
}

func (r *quicRelay) Process(conn *QUICConn, stream *QUICStream) (next bool) {
	return
}
