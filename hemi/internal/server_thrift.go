// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Thrift server implementation.

package internal

// ThriftServer is the interface for all Thrift servers.
// Users can implement their own Thrift server in exts, which implements the ThriftServer interface.
type ThriftServer interface {
	RPCServer

	RealServer() any // TODO
}
