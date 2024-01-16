// Copyright (c) 2020-2024 Sun Lei <valentine0401@163.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

package sunlei

import (
	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterServiceInit("sunlei", func(service *Service) error {
		return nil
	})
}
