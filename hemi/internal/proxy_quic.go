// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// QUIC proxy dealet implementation.

package internal

func init() {
	RegisterQUICDealet("quicProxy", func(name string, stage *Stage, mesher *QUICMesher) QUICDealet {
		d := new(quicProxy)
		d.onCreate(name, stage, mesher)
		return d
	})
}

// quicProxy relays QUIC connections to another QUIC server.
type quicProxy struct {
	// Mixins
	QUICDealet_
	proxy_
	// Assocs
	mesher *QUICMesher
	// States
}

func (d *quicProxy) onCreate(name string, stage *Stage, mesher *QUICMesher) {
	d.CompInit(name)
	d.proxy_.onCreate(stage)
	d.mesher = mesher
}
func (d *quicProxy) OnShutdown() {
	d.mesher.SubDone()
}

func (d *quicProxy) OnConfigure() {
	d.proxy_.onConfigure(d)
}
func (d *quicProxy) OnPrepare() {
	d.proxy_.onPrepare()
}

func (d *quicProxy) Deal(conn *QUICConn, stream *QUICStream) (next bool) {
	// TODO
	return
}
