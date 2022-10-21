// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Process for Linux.

package system

import (
	"syscall"
	"unsafe"
)

func DaemonSysAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Setsid: true,
	}
}
func DaemonInit() {
}

func SetAffinity(pid int, cpu int) bool {
	var cpus cpuset
	cpus.set(cpu)
	return schedSetAffinity(pid, &cpus)
}

type cpuset [16]uint64 // 1024bit

func (s *cpuset) set(cpu int) {
	if i := cpu / 64; i < len(s) {
		s[i] |= uint64(1 << (uint(cpu) % 64))
	}
}

func schedSetAffinity(pid int, set *cpuset) bool {
	_, _, e := syscall.RawSyscall(syscall.SYS_SCHED_SETAFFINITY, uintptr(pid), uintptr(unsafe.Sizeof(*set)), uintptr(unsafe.Pointer(set)))
	return e == 0
}
