// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Misc.

package common

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hexinfra/gorox/hemi/common/system"
)

var (
	DebugLevel int
	TargetAddr string
	CmdUIAddr  string
	WebUIAddr  string
	MyroxAddr  string
	ConfigFile string
	SingleMode bool
	DaemonMode bool
	BaseDir    string
	LogsDir    string
	TempDir    string
	VarsDir    string
	LogFile    string
	ErrFile    string
)

func GetConfig() (base string, file string) {
	if strings.HasPrefix(ConfigFile, "http://") || strings.HasPrefix(ConfigFile, "https://") {
		// base: scheme://host:port/prefix
		// file: /program.conf
		panic("currently not supported!")
	} else {
		if ConfigFile == "" {
			base = BaseDir
			file = "conf/" + Program + ".conf"
		} else if filepath.IsAbs(ConfigFile) { // /path/to/file.conf
			base = filepath.Dir(ConfigFile)
			file = filepath.Base(ConfigFile)
		} else { // path/to/file.conf
			base = BaseDir
			file = ConfigFile
		}
		base += "/"
	}
	return
}

var (
	Program string                                             // gorox, myrox, ...
	ExeArgs = append([]string{system.ExePath}, os.Args[1:]...) // /path/to/exe arg1 arg2 ...
)

const (
	CodeStop  = 10
	CodeCrash = 11
)

func Stop() {
	os.Exit(CodeStop)
}

func Crash(s string) {
	fmt.Fprintln(os.Stderr, s)
	os.Exit(CodeCrash)
}
