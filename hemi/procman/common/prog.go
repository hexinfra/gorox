// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Program related.

package common

import (
	"os"

	"github.com/hexinfra/gorox/hemi/common/system"
)

var (
	Program string                                             // gorox, myrox, ...
	ExeArgs = append([]string{system.ExePath}, os.Args[1:]...) // /path/to/exe arg1 arg2 ...
)
