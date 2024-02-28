// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Preloaded optional components.

package options

import (
	_ "github.com/hexinfra/gorox/hemi/options/backends/redis"
	_ "github.com/hexinfra/gorox/hemi/options/cachers/local"
	_ "github.com/hexinfra/gorox/hemi/options/cachers/mem"
	_ "github.com/hexinfra/gorox/hemi/options/complets/demo"
	_ "github.com/hexinfra/gorox/hemi/options/cronjobs/clean"
	_ "github.com/hexinfra/gorox/hemi/options/cronjobs/stat"
	_ "github.com/hexinfra/gorox/hemi/options/dealets/tcps/echo"
	_ "github.com/hexinfra/gorox/hemi/options/dealets/tcps/hello"
	_ "github.com/hexinfra/gorox/hemi/options/dealets/udps/dns"
	_ "github.com/hexinfra/gorox/hemi/options/handlets/access"
	_ "github.com/hexinfra/gorox/hemi/options/handlets/favicon"
	_ "github.com/hexinfra/gorox/hemi/options/handlets/hostname"
	_ "github.com/hexinfra/gorox/hemi/options/handlets/https"
	_ "github.com/hexinfra/gorox/hemi/options/handlets/limit"
	_ "github.com/hexinfra/gorox/hemi/options/handlets/referer"
	_ "github.com/hexinfra/gorox/hemi/options/handlets/rewriter"
	_ "github.com/hexinfra/gorox/hemi/options/handlets/sitex"
	_ "github.com/hexinfra/gorox/hemi/options/revisers/gunzip"
	_ "github.com/hexinfra/gorox/hemi/options/revisers/gzip"
	_ "github.com/hexinfra/gorox/hemi/options/revisers/head"
	_ "github.com/hexinfra/gorox/hemi/options/revisers/wrap"
	_ "github.com/hexinfra/gorox/hemi/options/servers/echo"
	_ "github.com/hexinfra/gorox/hemi/options/servers/socks"
	_ "github.com/hexinfra/gorox/hemi/options/socklets/hello"
	_ "github.com/hexinfra/gorox/hemi/options/staters/local"
)
