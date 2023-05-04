// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Import exts you need.

package exts

import ( // import contrib components, vendor exts, and your exts
	_ "github.com/hexinfra/gorox/hemi/contrib/backends/mysql"
	_ "github.com/hexinfra/gorox/hemi/contrib/backends/pgsql"
	_ "github.com/hexinfra/gorox/hemi/contrib/backends/redis"
	_ "github.com/hexinfra/gorox/hemi/contrib/cachers/local"
	_ "github.com/hexinfra/gorox/hemi/contrib/cachers/mem"
	_ "github.com/hexinfra/gorox/hemi/contrib/cachers/redis"
	_ "github.com/hexinfra/gorox/hemi/contrib/cronjobs/clean"
	_ "github.com/hexinfra/gorox/hemi/contrib/cronjobs/stat"
	_ "github.com/hexinfra/gorox/hemi/contrib/dealers/tcps/access"
	_ "github.com/hexinfra/gorox/hemi/contrib/dealers/tcps/echo"
	_ "github.com/hexinfra/gorox/hemi/contrib/dealers/tcps/hello"
	_ "github.com/hexinfra/gorox/hemi/contrib/dealers/tcps/mysql"
	_ "github.com/hexinfra/gorox/hemi/contrib/dealers/tcps/pgsql"
	_ "github.com/hexinfra/gorox/hemi/contrib/dealers/tcps/redis"
	_ "github.com/hexinfra/gorox/hemi/contrib/dealers/udps/dns"
	_ "github.com/hexinfra/gorox/hemi/contrib/editors/tcps/mysql"
	_ "github.com/hexinfra/gorox/hemi/contrib/editors/tcps/pgsql"
	_ "github.com/hexinfra/gorox/hemi/contrib/editors/tcps/redis"
	_ "github.com/hexinfra/gorox/hemi/contrib/handlets/access"
	_ "github.com/hexinfra/gorox/hemi/contrib/handlets/favicon"
	_ "github.com/hexinfra/gorox/hemi/contrib/handlets/hostname"
	_ "github.com/hexinfra/gorox/hemi/contrib/handlets/https"
	_ "github.com/hexinfra/gorox/hemi/contrib/handlets/limit"
	_ "github.com/hexinfra/gorox/hemi/contrib/handlets/mp4"
	_ "github.com/hexinfra/gorox/hemi/contrib/handlets/referer"
	_ "github.com/hexinfra/gorox/hemi/contrib/handlets/rewriter"
	_ "github.com/hexinfra/gorox/hemi/contrib/handlets/sitex"
	_ "github.com/hexinfra/gorox/hemi/contrib/handlets/webdav"
	_ "github.com/hexinfra/gorox/hemi/contrib/revisers/gunzip"
	_ "github.com/hexinfra/gorox/hemi/contrib/revisers/gzip"
	_ "github.com/hexinfra/gorox/hemi/contrib/revisers/head"
	_ "github.com/hexinfra/gorox/hemi/contrib/revisers/replace"
	_ "github.com/hexinfra/gorox/hemi/contrib/revisers/ssi"
	_ "github.com/hexinfra/gorox/hemi/contrib/revisers/wrap"
	_ "github.com/hexinfra/gorox/hemi/contrib/routers/simple"
	_ "github.com/hexinfra/gorox/hemi/contrib/servers/echo"
	_ "github.com/hexinfra/gorox/hemi/contrib/servers/socks"
	_ "github.com/hexinfra/gorox/hemi/contrib/socklets/hello"
	_ "github.com/hexinfra/gorox/hemi/contrib/staters/local"
	_ "github.com/hexinfra/gorox/hemi/contrib/staters/redis"
	_ "github.com/hexinfra/gorox/hemi/contrib/unitures/demo"
)