// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Builtin components are the standard components that supplement the core components of the Hemi Engine.

package builtin

import ( // preload all
	_ "github.com/hexinfra/gorox/hemi/builtin/backends/mysql"
	_ "github.com/hexinfra/gorox/hemi/builtin/backends/pgsql"
	_ "github.com/hexinfra/gorox/hemi/builtin/backends/redis"
	_ "github.com/hexinfra/gorox/hemi/builtin/cronjobs/clean"
	_ "github.com/hexinfra/gorox/hemi/builtin/dealets/tcpx/access"
	_ "github.com/hexinfra/gorox/hemi/builtin/dealets/tcpx/limit"
	_ "github.com/hexinfra/gorox/hemi/builtin/dealets/tcpx/mysql"
	_ "github.com/hexinfra/gorox/hemi/builtin/dealets/tcpx/pgsql"
	_ "github.com/hexinfra/gorox/hemi/builtin/dealets/tcpx/redis"
	_ "github.com/hexinfra/gorox/hemi/builtin/dealets/udpx/dns"
	_ "github.com/hexinfra/gorox/hemi/builtin/handlets/hostname"
	_ "github.com/hexinfra/gorox/hemi/builtin/handlets/https"
	_ "github.com/hexinfra/gorox/hemi/builtin/handlets/limit"
	_ "github.com/hexinfra/gorox/hemi/builtin/handlets/mp4"
	_ "github.com/hexinfra/gorox/hemi/builtin/handlets/sitex"
	_ "github.com/hexinfra/gorox/hemi/builtin/handlets/webdav"
	_ "github.com/hexinfra/gorox/hemi/builtin/hcaches/local"
	_ "github.com/hexinfra/gorox/hemi/builtin/hcaches/memory"
	_ "github.com/hexinfra/gorox/hemi/builtin/hcaches/redis"
	_ "github.com/hexinfra/gorox/hemi/builtin/hstates/local"
	_ "github.com/hexinfra/gorox/hemi/builtin/hstates/redis"
	_ "github.com/hexinfra/gorox/hemi/builtin/mappers/simple"
	_ "github.com/hexinfra/gorox/hemi/builtin/revisers/gunzip"
	_ "github.com/hexinfra/gorox/hemi/builtin/revisers/gzip"
	_ "github.com/hexinfra/gorox/hemi/builtin/revisers/head"
	_ "github.com/hexinfra/gorox/hemi/builtin/revisers/replace"
	_ "github.com/hexinfra/gorox/hemi/builtin/revisers/ssi"
	_ "github.com/hexinfra/gorox/hemi/builtin/revisers/wrap"
	_ "github.com/hexinfra/gorox/hemi/builtin/servers/echo"
	_ "github.com/hexinfra/gorox/hemi/builtin/servers/socks"
	_ "github.com/hexinfra/gorox/hemi/builtin/servers/tunnel"
	_ "github.com/hexinfra/gorox/hemi/builtin/socklets/hello"
)
