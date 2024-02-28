// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Import exts you need.

package exts

import ( // import optional components, vendor exts, and your exts
	_ "github.com/hexinfra/gorox/hemi/options/backends/mongo"
	_ "github.com/hexinfra/gorox/hemi/options/backends/mysql"
	_ "github.com/hexinfra/gorox/hemi/options/backends/pgsql"
	_ "github.com/hexinfra/gorox/hemi/options/backends/redis"
	_ "github.com/hexinfra/gorox/hemi/options/cachers/local"
	_ "github.com/hexinfra/gorox/hemi/options/cachers/mem"
	_ "github.com/hexinfra/gorox/hemi/options/cachers/redis"
	_ "github.com/hexinfra/gorox/hemi/options/complets/demo"
	_ "github.com/hexinfra/gorox/hemi/options/cronjobs/clean"
	_ "github.com/hexinfra/gorox/hemi/options/cronjobs/stat"
	_ "github.com/hexinfra/gorox/hemi/options/dealets/tcps/access"
	_ "github.com/hexinfra/gorox/hemi/options/dealets/tcps/echo"
	_ "github.com/hexinfra/gorox/hemi/options/dealets/tcps/hello"
	_ "github.com/hexinfra/gorox/hemi/options/dealets/tcps/mongo"
	_ "github.com/hexinfra/gorox/hemi/options/dealets/tcps/mysql"
	_ "github.com/hexinfra/gorox/hemi/options/dealets/tcps/pgsql"
	_ "github.com/hexinfra/gorox/hemi/options/dealets/tcps/redis"
	_ "github.com/hexinfra/gorox/hemi/options/dealets/udps/dns"
	_ "github.com/hexinfra/gorox/hemi/options/handlets/access"
	_ "github.com/hexinfra/gorox/hemi/options/handlets/favicon"
	_ "github.com/hexinfra/gorox/hemi/options/handlets/hostname"
	_ "github.com/hexinfra/gorox/hemi/options/handlets/https"
	_ "github.com/hexinfra/gorox/hemi/options/handlets/limit"
	_ "github.com/hexinfra/gorox/hemi/options/handlets/mp4"
	_ "github.com/hexinfra/gorox/hemi/options/handlets/referer"
	_ "github.com/hexinfra/gorox/hemi/options/handlets/rewriter"
	_ "github.com/hexinfra/gorox/hemi/options/handlets/sitex"
	_ "github.com/hexinfra/gorox/hemi/options/handlets/webdav"
	_ "github.com/hexinfra/gorox/hemi/options/mappers/simple"
	_ "github.com/hexinfra/gorox/hemi/options/revisers/gunzip"
	_ "github.com/hexinfra/gorox/hemi/options/revisers/gzip"
	_ "github.com/hexinfra/gorox/hemi/options/revisers/head"
	_ "github.com/hexinfra/gorox/hemi/options/revisers/replace"
	_ "github.com/hexinfra/gorox/hemi/options/revisers/ssi"
	_ "github.com/hexinfra/gorox/hemi/options/revisers/wrap"
	_ "github.com/hexinfra/gorox/hemi/options/servers/echo"
	_ "github.com/hexinfra/gorox/hemi/options/servers/socks"
	_ "github.com/hexinfra/gorox/hemi/options/socklets/hello"
	_ "github.com/hexinfra/gorox/hemi/options/staters/local"
	_ "github.com/hexinfra/gorox/hemi/options/staters/redis"
)
