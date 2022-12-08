// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

package exts

import ( // all standard components
	_ "github.com/hexinfra/gorox/hemi/standard/cachers/local"
	_ "github.com/hexinfra/gorox/hemi/standard/cachers/redis"
	_ "github.com/hexinfra/gorox/hemi/standard/cronjobs/clean"
	_ "github.com/hexinfra/gorox/hemi/standard/cronjobs/stat"
	_ "github.com/hexinfra/gorox/hemi/standard/filters/tcps/mysql"
	_ "github.com/hexinfra/gorox/hemi/standard/filters/tcps/redis"
	_ "github.com/hexinfra/gorox/hemi/standard/handlers/access"
	_ "github.com/hexinfra/gorox/hemi/standard/handlers/ajp"
	_ "github.com/hexinfra/gorox/hemi/standard/handlers/favicon"
	_ "github.com/hexinfra/gorox/hemi/standard/handlers/fcgi"
	_ "github.com/hexinfra/gorox/hemi/standard/handlers/gatex"
	_ "github.com/hexinfra/gorox/hemi/standard/handlers/hostname"
	_ "github.com/hexinfra/gorox/hemi/standard/handlers/https"
	_ "github.com/hexinfra/gorox/hemi/standard/handlers/limit"
	_ "github.com/hexinfra/gorox/hemi/standard/handlers/referer"
	_ "github.com/hexinfra/gorox/hemi/standard/handlers/rewrite"
	_ "github.com/hexinfra/gorox/hemi/standard/handlers/sitex"
	_ "github.com/hexinfra/gorox/hemi/standard/handlers/uwsgi"
	_ "github.com/hexinfra/gorox/hemi/standard/optwares/demo"
	_ "github.com/hexinfra/gorox/hemi/standard/revisers/gzip"
	_ "github.com/hexinfra/gorox/hemi/standard/revisers/head"
	_ "github.com/hexinfra/gorox/hemi/standard/revisers/replace"
	_ "github.com/hexinfra/gorox/hemi/standard/revisers/ssi"
	_ "github.com/hexinfra/gorox/hemi/standard/revisers/wrap"
	_ "github.com/hexinfra/gorox/hemi/standard/runners/tcps/echo"
	_ "github.com/hexinfra/gorox/hemi/standard/runners/tcps/hello"
	_ "github.com/hexinfra/gorox/hemi/standard/runners/udps/dns"
	_ "github.com/hexinfra/gorox/hemi/standard/servers/echo"
	_ "github.com/hexinfra/gorox/hemi/standard/servers/socks"
	_ "github.com/hexinfra/gorox/hemi/standard/socklets/hello"
	_ "github.com/hexinfra/gorox/hemi/standard/staters/local"
	_ "github.com/hexinfra/gorox/hemi/standard/staters/redis"
)
