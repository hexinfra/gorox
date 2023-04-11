// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// General HRPC server implementation.

package internal

// hrpcServer
type hrpcServer interface {
	httpServer

	linkSvcs()
	findSvc(hostname []byte) *Svc
}
