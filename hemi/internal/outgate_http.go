// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// General HTTP outgate.

package internal

// httpOutgate_ is the mixin for HTTP[1-3]Outgate.
type httpOutgate_ struct {
	// Mixins
	outgate_
	httpClient_
	// States
}

func (f *httpOutgate_) init(name string, stage *Stage) {
	f.outgate_.init(name, stage)
}

func (f *httpOutgate_) onConfigure() {
	f.outgate_.onConfigure()
	// maxStreamsPerConn
	f.ConfigureInt32("maxStreamsPerConn", &f.maxStreamsPerConn, func(value int32) bool { return value > 0 }, 1000)
	// saveContentFilesDir
	f.ConfigureString("saveContentFilesDir", &f.saveContentFilesDir, func(value string) bool { return value != "" }, TempDir()+"/outgates/"+f.name)
	// maxContentSize
	f.ConfigureInt64("maxContentSize", &f.maxContentSize, func(value int64) bool { return value > 0 }, _1T)
}
func (f *httpOutgate_) onPrepare() {
	f.outgate_.onPrepare()
	f.makeContentFilesDir(0755)
}
func (f *httpOutgate_) onShutdown() {
	f.outgate_.onShutdown()
}
