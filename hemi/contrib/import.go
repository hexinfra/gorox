// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Preloaded components.

package contrib

import (
	_ "github.com/hexinfra/gorox/hemi/contrib/cachers/local"
	_ "github.com/hexinfra/gorox/hemi/contrib/cronjobs/clean"
	_ "github.com/hexinfra/gorox/hemi/contrib/cronjobs/stat"
	_ "github.com/hexinfra/gorox/hemi/contrib/dealets/tcps/echo"
	_ "github.com/hexinfra/gorox/hemi/contrib/dealets/tcps/hello"
	_ "github.com/hexinfra/gorox/hemi/contrib/dealets/udps/dns"
	_ "github.com/hexinfra/gorox/hemi/contrib/editors/tcps/mysql"
	_ "github.com/hexinfra/gorox/hemi/contrib/handlets/access"
	_ "github.com/hexinfra/gorox/hemi/contrib/handlets/favicon"
	_ "github.com/hexinfra/gorox/hemi/contrib/handlets/hostname"
	_ "github.com/hexinfra/gorox/hemi/contrib/handlets/https"
	_ "github.com/hexinfra/gorox/hemi/contrib/handlets/limit"
	_ "github.com/hexinfra/gorox/hemi/contrib/handlets/referer"
	_ "github.com/hexinfra/gorox/hemi/contrib/handlets/rewriter"
	_ "github.com/hexinfra/gorox/hemi/contrib/handlets/sitex"
	_ "github.com/hexinfra/gorox/hemi/contrib/revisers/gzip"
	_ "github.com/hexinfra/gorox/hemi/contrib/revisers/head"
	_ "github.com/hexinfra/gorox/hemi/contrib/revisers/wrap"
	_ "github.com/hexinfra/gorox/hemi/contrib/servers/echo"
	_ "github.com/hexinfra/gorox/hemi/contrib/servers/socks"
	_ "github.com/hexinfra/gorox/hemi/contrib/socklets/hello"
	_ "github.com/hexinfra/gorox/hemi/contrib/staters/local"
	_ "github.com/hexinfra/gorox/hemi/contrib/unitures/demo"
)
