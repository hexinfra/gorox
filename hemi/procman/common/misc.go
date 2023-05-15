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
	Config     string
	SingleMode bool
	DaemonMode bool
	LogFile    string
	BaseDir    string
	LogsDir    string
	TempDir    string
	VarsDir    string
)

func GetConfig() (base string, file string) {
	baseDir, config := BaseDir, Config
	if strings.HasPrefix(config, "http://") || strings.HasPrefix(config, "https://") {
		// base: scheme://host:port/prefix
		// file: /program.conf
		panic("currently not supported!")
	} else {
		if config == "" {
			base = baseDir
			file = "conf/" + Program + ".conf"
		} else if filepath.IsAbs(config) { // /path/to/file.conf
			base = filepath.Dir(config)
			file = filepath.Base(config)
		} else { // path/to/file.conf
			base = baseDir
			file = config
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
