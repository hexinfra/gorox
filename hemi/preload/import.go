// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Preloaded components.

package preload

import ( // all preload components are preloaded
	_ "github.com/hexinfra/gorox/hemi/preload/cachers/local"
	_ "github.com/hexinfra/gorox/hemi/preload/cronjobs/clean"
	_ "github.com/hexinfra/gorox/hemi/preload/cronjobs/stat"
	_ "github.com/hexinfra/gorox/hemi/preload/dealets/tcps/echo"
	_ "github.com/hexinfra/gorox/hemi/preload/dealets/tcps/hello"
	_ "github.com/hexinfra/gorox/hemi/preload/dealets/udps/dns"
	_ "github.com/hexinfra/gorox/hemi/preload/editors/tcps/mysql"
	_ "github.com/hexinfra/gorox/hemi/preload/handlets/access"
	_ "github.com/hexinfra/gorox/hemi/preload/handlets/favicon"
	_ "github.com/hexinfra/gorox/hemi/preload/handlets/hostname"
	_ "github.com/hexinfra/gorox/hemi/preload/handlets/https"
	_ "github.com/hexinfra/gorox/hemi/preload/handlets/limit"
	_ "github.com/hexinfra/gorox/hemi/preload/handlets/referer"
	_ "github.com/hexinfra/gorox/hemi/preload/handlets/rewrite"
	_ "github.com/hexinfra/gorox/hemi/preload/handlets/sitex"
	_ "github.com/hexinfra/gorox/hemi/preload/revisers/gzip"
	_ "github.com/hexinfra/gorox/hemi/preload/revisers/head"
	_ "github.com/hexinfra/gorox/hemi/preload/revisers/wrap"
	_ "github.com/hexinfra/gorox/hemi/preload/servers/echo"
	_ "github.com/hexinfra/gorox/hemi/preload/socklets/hello"
	_ "github.com/hexinfra/gorox/hemi/preload/staters/local"
	_ "github.com/hexinfra/gorox/hemi/preload/unitures/demo"
)
