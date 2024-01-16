// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// This is a hello service showing how to use Gorox RPC Framework to host a service.

package hello

import (
	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	// Register service initializer.
	RegisterServiceInit("hello", func(service *Service) error {
		return nil
	})
}
