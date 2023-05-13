// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Config fetching.

package common

import (
	"path/filepath"
	"strings"
)

func GetConfig() (base string, file string) {
	baseDir, config := *BaseDir, *Config
	if strings.HasPrefix(config, "http://") || strings.HasPrefix(config, "https://") {
		// base: scheme://host:port/path/
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
