// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Misc types and functions for FreeBSD.

package system

import (
	"fmt"
)

func Check() bool {
	// TODO: freebsd version >= 12.0
	return true
}

func Advise() {
	fmt.Println("not implemented")
}
