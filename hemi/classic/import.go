// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Classic components are the standard builtin components that supplement the core components of the Hemi Engine.

package classic

import ( // preload all
	_ "github.com/hexinfra/gorox/hemi/classic/backends/mysql"
	_ "github.com/hexinfra/gorox/hemi/classic/backends/pgsql"
	_ "github.com/hexinfra/gorox/hemi/classic/backends/redis"
	_ "github.com/hexinfra/gorox/hemi/classic/cronjobs/clean"
	_ "github.com/hexinfra/gorox/hemi/classic/dealets/tcpx/access"
	_ "github.com/hexinfra/gorox/hemi/classic/dealets/tcpx/limit"
	_ "github.com/hexinfra/gorox/hemi/classic/dealets/tcpx/mysql"
	_ "github.com/hexinfra/gorox/hemi/classic/dealets/tcpx/pgsql"
	_ "github.com/hexinfra/gorox/hemi/classic/dealets/tcpx/redis"
	_ "github.com/hexinfra/gorox/hemi/classic/dealets/udpx/dns"
	_ "github.com/hexinfra/gorox/hemi/classic/handlets/hostname"
	_ "github.com/hexinfra/gorox/hemi/classic/handlets/https"
	_ "github.com/hexinfra/gorox/hemi/classic/handlets/limit"
	_ "github.com/hexinfra/gorox/hemi/classic/handlets/mp4"
	_ "github.com/hexinfra/gorox/hemi/classic/handlets/sitex"
	_ "github.com/hexinfra/gorox/hemi/classic/handlets/webdav"
	_ "github.com/hexinfra/gorox/hemi/classic/hcaches/local"
	_ "github.com/hexinfra/gorox/hemi/classic/hcaches/memory"
	_ "github.com/hexinfra/gorox/hemi/classic/hcaches/redis"
	_ "github.com/hexinfra/gorox/hemi/classic/hstates/local"
	_ "github.com/hexinfra/gorox/hemi/classic/hstates/redis"
	_ "github.com/hexinfra/gorox/hemi/classic/mappers/simple"
	_ "github.com/hexinfra/gorox/hemi/classic/revisers/gunzip"
	_ "github.com/hexinfra/gorox/hemi/classic/revisers/gzip"
	_ "github.com/hexinfra/gorox/hemi/classic/revisers/head"
	_ "github.com/hexinfra/gorox/hemi/classic/revisers/replace"
	_ "github.com/hexinfra/gorox/hemi/classic/revisers/ssi"
	_ "github.com/hexinfra/gorox/hemi/classic/revisers/wrap"
	_ "github.com/hexinfra/gorox/hemi/classic/servers/echo"
	_ "github.com/hexinfra/gorox/hemi/classic/servers/socks"
	_ "github.com/hexinfra/gorox/hemi/classic/servers/tunnel"
	_ "github.com/hexinfra/gorox/hemi/classic/socklets/hello"
)
