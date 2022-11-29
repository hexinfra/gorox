// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// QUIC proxy runner implementation.

package internal

func init() {
	RegisterQUICRunner("quicProxy", func(name string, stage *Stage, mesher *QUICMesher) QUICRunner {
		r := new(quicProxy)
		r.onCreate(name, stage, mesher)
		return r
	})
}

// quicProxy passes QUIC connections to another QUIC server.
type quicProxy struct {
	// Mixins
	QUICRunner_
	proxy_
	// Assocs
	mesher *QUICMesher
	// States
}

func (r *quicProxy) onCreate(name string, stage *Stage, mesher *QUICMesher) {
	r.CompInit(name)
	r.proxy_.onCreate(stage)
	r.mesher = mesher
}

func (r *quicProxy) OnConfigure() {
	r.proxy_.onConfigure(r)
}
func (r *quicProxy) OnPrepare() {
	r.proxy_.onPrepare()
}

func (r *quicProxy) OnShutdown() {
	r.mesher.SubDone()
}

func (r *quicProxy) Process(conn *QUICConn, stream *QUICStream) (next bool) {
	// TODO
	return
}
