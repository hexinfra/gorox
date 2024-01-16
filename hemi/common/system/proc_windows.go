// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Process for Windows.

package system

import (
	"syscall"
)

var kernel32 = syscall.MustLoadDLL("kernel32.dll")

func DaemonSysAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		HideWindow: true,
	}
}
func DaemonInit() {
	kernel32.MustFindProc("FreeConsole").Call()
}

func SetAffinity(pid int, cpu int) bool {
	return true
}
