// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

package alex

import (
	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterSvcInit("alex", func(svc *Svc) error {
		return nil
	})
}