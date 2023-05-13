// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Flags.

package common

import (
	"flag"
)

var (
	DebugLevel int
	TargetAddr string
	AdminAddr  string
	MyroxAddr  = flag.String("myrox", "", "")
	Config     = flag.String("conf", "", "")
	SingleMode = flag.Bool("single", false, "")
	DaemonMode = flag.Bool("daemon", false, "")
	LogFile    = flag.String("log", "", "")
	BaseDir    = flag.String("base", "", "")
	LogsDir    = flag.String("logs", "", "")
	TempDir    = flag.String("temp", "", "")
	VarsDir    = flag.String("vars", "", "")
)
