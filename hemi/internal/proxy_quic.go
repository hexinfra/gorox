// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// QUIC proxy filter implementation.

package internal

func init() {
	RegisterQUICFilter("quicProxy", func(name string, stage *Stage, mesher *QUICMesher) QUICFilter {
		f := new(quicProxy)
		f.onCreate(name, stage, mesher)
		return f
	})
}

// quicProxy relays QUIC connections to another QUIC server.
type quicProxy struct {
	// Mixins
	QUICFilter_
	proxy_
	// Assocs
	mesher *QUICMesher
	// States
}

func (f *quicProxy) onCreate(name string, stage *Stage, mesher *QUICMesher) {
	f.MakeComp(name)
	f.proxy_.onCreate(stage)
	f.mesher = mesher
}
func (f *quicProxy) OnShutdown() {
	f.mesher.SubDone()
}

func (f *quicProxy) OnConfigure() {
	f.proxy_.onConfigure(f)
}
func (f *quicProxy) OnPrepare() {
	f.proxy_.onPrepare(f)
}

func (f *quicProxy) Deal(conn *QUICConn, stream *QUICStream) (next bool) {
	// TODO
	// NOTE: if configured as forward proxy, work as a SOCKS server? HTTP tunnel?
	return
}
