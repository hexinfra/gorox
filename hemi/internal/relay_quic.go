// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// QUIC relay filter implementation.

package internal

func init() {
	RegisterQUICFilter("quicRelay", func(name string, stage *Stage, mesher *QUICMesher) QUICFilter {
		f := new(quicRelay)
		f.onCreate(name, stage, mesher)
		return f
	})
}

// quicRelay relays QUIC connections to another QUIC server.
type quicRelay struct {
	// Mixins
	QUICFilter_
	proxy_
	// Assocs
	mesher *QUICMesher
	// States
}

func (f *quicRelay) onCreate(name string, stage *Stage, mesher *QUICMesher) {
	f.MakeComp(name)
	f.proxy_.onCreate(stage)
	f.mesher = mesher
}
func (f *quicRelay) OnShutdown() {
	f.mesher.SubDone()
}

func (f *quicRelay) OnConfigure() {
	f.proxy_.onConfigure(f)
}
func (f *quicRelay) OnPrepare() {
	f.proxy_.onPrepare(f)
}

func (f *quicRelay) Deal(conn *QUICConn, stream *QUICStream) (next bool) {
	// TODO
	return
}
