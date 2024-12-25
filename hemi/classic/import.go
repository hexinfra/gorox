// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Classic components.

package classic

import ( // preload all
	_ "github.com/hexinfra/gorox/hemi/classic/backends/mongo"
	_ "github.com/hexinfra/gorox/hemi/classic/backends/mysql"
	_ "github.com/hexinfra/gorox/hemi/classic/backends/pgsql"
	_ "github.com/hexinfra/gorox/hemi/classic/backends/redis"
	_ "github.com/hexinfra/gorox/hemi/classic/cachers/local"
	_ "github.com/hexinfra/gorox/hemi/classic/cachers/mem"
	_ "github.com/hexinfra/gorox/hemi/classic/cachers/redis"
	_ "github.com/hexinfra/gorox/hemi/classic/cronjobs/clean"
	_ "github.com/hexinfra/gorox/hemi/classic/cronjobs/stat"
	_ "github.com/hexinfra/gorox/hemi/classic/dealets/tcpx/access"
	_ "github.com/hexinfra/gorox/hemi/classic/dealets/tcpx/limit"
	_ "github.com/hexinfra/gorox/hemi/classic/dealets/tcpx/mongo"
	_ "github.com/hexinfra/gorox/hemi/classic/dealets/tcpx/mysql"
	_ "github.com/hexinfra/gorox/hemi/classic/dealets/tcpx/pgsql"
	_ "github.com/hexinfra/gorox/hemi/classic/dealets/tcpx/redis"
	_ "github.com/hexinfra/gorox/hemi/classic/dealets/udpx/dns"
	_ "github.com/hexinfra/gorox/hemi/classic/handlets/favicon"
	_ "github.com/hexinfra/gorox/hemi/classic/handlets/hostname"
	_ "github.com/hexinfra/gorox/hemi/classic/handlets/https"
	_ "github.com/hexinfra/gorox/hemi/classic/handlets/limit"
	_ "github.com/hexinfra/gorox/hemi/classic/handlets/mp4"
	_ "github.com/hexinfra/gorox/hemi/classic/handlets/rewriter"
	_ "github.com/hexinfra/gorox/hemi/classic/handlets/sitex"
	_ "github.com/hexinfra/gorox/hemi/classic/handlets/webdav"
	_ "github.com/hexinfra/gorox/hemi/classic/mappers/simple"
	_ "github.com/hexinfra/gorox/hemi/classic/revisers/gunzip"
	_ "github.com/hexinfra/gorox/hemi/classic/revisers/gzip"
	_ "github.com/hexinfra/gorox/hemi/classic/revisers/head"
	_ "github.com/hexinfra/gorox/hemi/classic/revisers/replace"
	_ "github.com/hexinfra/gorox/hemi/classic/revisers/ssi"
	_ "github.com/hexinfra/gorox/hemi/classic/revisers/wrap"
	_ "github.com/hexinfra/gorox/hemi/classic/servers/echo"
	_ "github.com/hexinfra/gorox/hemi/classic/servers/socks"
	_ "github.com/hexinfra/gorox/hemi/classic/servers/tcpoh"
	_ "github.com/hexinfra/gorox/hemi/classic/servers/udpoh"
	_ "github.com/hexinfra/gorox/hemi/classic/socklets/hello"
	_ "github.com/hexinfra/gorox/hemi/classic/staters/local"
	_ "github.com/hexinfra/gorox/hemi/classic/staters/redis"
)
