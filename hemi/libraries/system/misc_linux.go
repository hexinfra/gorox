// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Misc types and functions for Linux.

package system

import (
	"fmt"
	"runtime"
	"syscall"
)

var (
	EINTR  = syscall.EINTR
	EAGAIN = syscall.EAGAIN
)

var (
	errEAGAIN error = syscall.EAGAIN
	errEINVAL error = syscall.EINVAL
	errENOENT error = syscall.ENOENT
)

func errnoErr(e syscall.Errno) error {
	switch e {
	case 0:
		return nil
	case syscall.EAGAIN:
		return errEAGAIN
	case syscall.EINVAL:
		return errEINVAL
	case syscall.ENOENT:
		return errENOENT
	}
	return e
}

func ignoringEINTRIO(fn func(fd int, p []byte) (int, error), fd int, p []byte) (int, error) {
	for {
		n, err := fn(fd, p)
		if err != EINTR {
			return n, err
		}
	}
}

func Check() bool {
	if runtime.GOARCH != "amd64" {
		return false
	}
	major, minor := kernelVersion()
	if major > 3 || major == 3 && minor >= 9 {
		return true
	}
	return true
}

func Advise() {
	fmt.Println("not implemented")
}

func kernelVersion() (major int, minor int) {
	var uname syscall.Utsname
	if err := syscall.Uname(&uname); err != nil {
		return
	}

	rl := uname.Release
	var values [2]int
	vi := 0
	value := 0
	for _, c := range rl {
		if '0' <= c && c <= '9' {
			value = (value * 10) + int(c-'0')
		} else {
			// Note that we're assuming N.N.N here.  If we see anything else we are likely to
			// mis-parse it.
			values[vi] = value
			vi++
			if vi >= len(values) {
				break
			}
			value = 0
		}
	}
	switch vi {
	case 0:
		return 0, 0
	case 1:
		return values[0], 0
	case 2:
		return values[0], values[1]
	}
	return
}
