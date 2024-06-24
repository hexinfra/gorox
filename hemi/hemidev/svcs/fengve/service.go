// Copyright (c) 2020-2024 Feng Wei <feng19910104@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

package fengve

import (
	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterServiceInit("fengve", func(service *Service) error {
		return nil
	})
}
