// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

package hemisvc

import (
	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterServiceInit("hemisvc", func(service *Service) error {
		return nil
	})
}
