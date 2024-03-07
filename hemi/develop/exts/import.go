// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Import exts you need.

package exts

import _ "github.com/hexinfra/gorox/hemi/addons"

import ( // import extra addons here
	_ "github.com/hexinfra/gorox/hemi/addons/backends/mongo"
	_ "github.com/hexinfra/gorox/hemi/addons/backends/mysql"
	_ "github.com/hexinfra/gorox/hemi/addons/backends/pgsql"
	_ "github.com/hexinfra/gorox/hemi/addons/backends/redis"
	_ "github.com/hexinfra/gorox/hemi/addons/cachers/local"
	_ "github.com/hexinfra/gorox/hemi/addons/cachers/mem"
	_ "github.com/hexinfra/gorox/hemi/addons/cachers/redis"
	_ "github.com/hexinfra/gorox/hemi/addons/cronjobs/clean"
	_ "github.com/hexinfra/gorox/hemi/addons/cronjobs/stat"
	_ "github.com/hexinfra/gorox/hemi/addons/dealets/tcps/access"
	_ "github.com/hexinfra/gorox/hemi/addons/dealets/tcps/echo"
	_ "github.com/hexinfra/gorox/hemi/addons/dealets/tcps/hello"
	_ "github.com/hexinfra/gorox/hemi/addons/dealets/tcps/mongo"
	_ "github.com/hexinfra/gorox/hemi/addons/dealets/tcps/mysql"
	_ "github.com/hexinfra/gorox/hemi/addons/dealets/tcps/pgsql"
	_ "github.com/hexinfra/gorox/hemi/addons/dealets/tcps/redis"
	_ "github.com/hexinfra/gorox/hemi/addons/dealets/udps/dns"
	_ "github.com/hexinfra/gorox/hemi/addons/handlets/access"
	_ "github.com/hexinfra/gorox/hemi/addons/handlets/favicon"
	_ "github.com/hexinfra/gorox/hemi/addons/handlets/hostname"
	_ "github.com/hexinfra/gorox/hemi/addons/handlets/https"
	_ "github.com/hexinfra/gorox/hemi/addons/handlets/limit"
	_ "github.com/hexinfra/gorox/hemi/addons/handlets/mp4"
	_ "github.com/hexinfra/gorox/hemi/addons/handlets/referer"
	_ "github.com/hexinfra/gorox/hemi/addons/handlets/rewriter"
	_ "github.com/hexinfra/gorox/hemi/addons/handlets/sitex"
	_ "github.com/hexinfra/gorox/hemi/addons/handlets/webdav"
	_ "github.com/hexinfra/gorox/hemi/addons/mappers/simple"
	_ "github.com/hexinfra/gorox/hemi/addons/revisers/gunzip"
	_ "github.com/hexinfra/gorox/hemi/addons/revisers/gzip"
	_ "github.com/hexinfra/gorox/hemi/addons/revisers/head"
	_ "github.com/hexinfra/gorox/hemi/addons/revisers/replace"
	_ "github.com/hexinfra/gorox/hemi/addons/revisers/ssi"
	_ "github.com/hexinfra/gorox/hemi/addons/revisers/wrap"
	_ "github.com/hexinfra/gorox/hemi/addons/servers/echo"
	_ "github.com/hexinfra/gorox/hemi/addons/servers/ipoh"
	_ "github.com/hexinfra/gorox/hemi/addons/servers/socks"
	_ "github.com/hexinfra/gorox/hemi/addons/servers/tcpoh"
	_ "github.com/hexinfra/gorox/hemi/addons/servers/udpoh"
	_ "github.com/hexinfra/gorox/hemi/addons/socklets/hello"
	_ "github.com/hexinfra/gorox/hemi/addons/staters/local"
	_ "github.com/hexinfra/gorox/hemi/addons/staters/redis"
)

import ( // import vendor exts here
)

import ( // import your exts here
)
