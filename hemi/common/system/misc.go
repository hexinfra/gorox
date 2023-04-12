// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Auxiliary types and functions of the operating system.

package system

import (
	"os"
	"path/filepath"
	"runtime"
)

var (
	ExePath string
	ExeDir  string
)

func init() {
	// set public variables
	path, err := os.Executable()
	if err != nil {
		panic(err)
	}
	ExePath = path
	ExeDir = filepath.Dir(path)
	if runtime.GOOS == "windows" { // change '\\' to '/'
		ExePath = filepath.ToSlash(ExePath)
		ExeDir = filepath.ToSlash(ExeDir)
	}
}
