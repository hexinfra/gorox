// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

package exts

import ( // all contrib components
	_ "github.com/hexinfra/gorox/hemi/contrib/cachers/local"
	_ "github.com/hexinfra/gorox/hemi/contrib/cachers/redis"
	_ "github.com/hexinfra/gorox/hemi/contrib/cronjobs/clean"
	_ "github.com/hexinfra/gorox/hemi/contrib/cronjobs/stat"
	_ "github.com/hexinfra/gorox/hemi/contrib/filters/tcps/mysql"
	_ "github.com/hexinfra/gorox/hemi/contrib/filters/tcps/redis"
	_ "github.com/hexinfra/gorox/hemi/contrib/handlers/access"
	_ "github.com/hexinfra/gorox/hemi/contrib/handlers/ajp"
	_ "github.com/hexinfra/gorox/hemi/contrib/handlers/favicon"
	_ "github.com/hexinfra/gorox/hemi/contrib/handlers/fcgi"
	_ "github.com/hexinfra/gorox/hemi/contrib/handlers/gatex"
	_ "github.com/hexinfra/gorox/hemi/contrib/handlers/hostname"
	_ "github.com/hexinfra/gorox/hemi/contrib/handlers/https"
	_ "github.com/hexinfra/gorox/hemi/contrib/handlers/limit"
	_ "github.com/hexinfra/gorox/hemi/contrib/handlers/referer"
	_ "github.com/hexinfra/gorox/hemi/contrib/handlers/rewrite"
	_ "github.com/hexinfra/gorox/hemi/contrib/handlers/sitex"
	_ "github.com/hexinfra/gorox/hemi/contrib/handlers/uwsgi"
	_ "github.com/hexinfra/gorox/hemi/contrib/optwares/demo"
	_ "github.com/hexinfra/gorox/hemi/contrib/revisers/gzip"
	_ "github.com/hexinfra/gorox/hemi/contrib/revisers/head"
	_ "github.com/hexinfra/gorox/hemi/contrib/revisers/replace"
	_ "github.com/hexinfra/gorox/hemi/contrib/revisers/ssi"
	_ "github.com/hexinfra/gorox/hemi/contrib/revisers/wrap"
	_ "github.com/hexinfra/gorox/hemi/contrib/runners/tcps/echo"
	_ "github.com/hexinfra/gorox/hemi/contrib/runners/tcps/hello"
	_ "github.com/hexinfra/gorox/hemi/contrib/runners/udps/dns"
	_ "github.com/hexinfra/gorox/hemi/contrib/servers/echo"
	_ "github.com/hexinfra/gorox/hemi/contrib/servers/socks"
	_ "github.com/hexinfra/gorox/hemi/contrib/socklets/hello"
	_ "github.com/hexinfra/gorox/hemi/contrib/staters/local"
	_ "github.com/hexinfra/gorox/hemi/contrib/staters/redis"
)
