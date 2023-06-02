// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Thrift server bridge.

package internal

// ThriftBridge is the interface for all Thrift server bridges.
// Users can implement their own Thrift server in exts, which may embeds thrift.TServer and must implements the ThriftBridge interface.
type ThriftBridge interface {
	// Imports
	rpcServer
	// Methods
	ThriftServer() any // may be a thrift.TServer?
}
