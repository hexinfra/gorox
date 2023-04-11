// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// This is a hello svc showing how to use Gorox RPC server to host a svc.

package hello

import (
	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	// Register svc initializer.
	RegisterSvcInit("hello", func(svc *Svc) error {
		return nil
	})
}
