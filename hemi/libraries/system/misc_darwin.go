// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Misc types and functions for macOS.

package system

import (
	"fmt"
)

var (
	EINTR  = error(nil) // not used
	EAGAIN = error(nil) // not used
)

func Check() bool {
	// ensure reuseport
	return true
}

func Advise() {
	fmt.Println("not implemented")
}
