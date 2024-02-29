// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Preloaded addons.

package addons

import (
	_ "github.com/hexinfra/gorox/hemi/addons/backends/redis"
	_ "github.com/hexinfra/gorox/hemi/addons/cachers/local"
	_ "github.com/hexinfra/gorox/hemi/addons/cachers/mem"
	_ "github.com/hexinfra/gorox/hemi/addons/complets/demo"
	_ "github.com/hexinfra/gorox/hemi/addons/cronjobs/clean"
	_ "github.com/hexinfra/gorox/hemi/addons/cronjobs/stat"
	_ "github.com/hexinfra/gorox/hemi/addons/dealets/tcps/echo"
	_ "github.com/hexinfra/gorox/hemi/addons/dealets/tcps/hello"
	_ "github.com/hexinfra/gorox/hemi/addons/dealets/udps/dns"
	_ "github.com/hexinfra/gorox/hemi/addons/handlets/access"
	_ "github.com/hexinfra/gorox/hemi/addons/handlets/favicon"
	_ "github.com/hexinfra/gorox/hemi/addons/handlets/hostname"
	_ "github.com/hexinfra/gorox/hemi/addons/handlets/https"
	_ "github.com/hexinfra/gorox/hemi/addons/handlets/limit"
	_ "github.com/hexinfra/gorox/hemi/addons/handlets/referer"
	_ "github.com/hexinfra/gorox/hemi/addons/handlets/rewriter"
	_ "github.com/hexinfra/gorox/hemi/addons/handlets/sitex"
	_ "github.com/hexinfra/gorox/hemi/addons/revisers/gunzip"
	_ "github.com/hexinfra/gorox/hemi/addons/revisers/gzip"
	_ "github.com/hexinfra/gorox/hemi/addons/revisers/head"
	_ "github.com/hexinfra/gorox/hemi/addons/revisers/wrap"
	_ "github.com/hexinfra/gorox/hemi/addons/servers/echo"
	_ "github.com/hexinfra/gorox/hemi/addons/servers/socks"
	_ "github.com/hexinfra/gorox/hemi/addons/socklets/hello"
	_ "github.com/hexinfra/gorox/hemi/addons/staters/local"
)
