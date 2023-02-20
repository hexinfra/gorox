// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Standard components.

package standard

import ( // all standard components
	_ "github.com/hexinfra/gorox/hemi/standard/cachers/local"
	_ "github.com/hexinfra/gorox/hemi/standard/cronjobs/clean"
	_ "github.com/hexinfra/gorox/hemi/standard/cronjobs/stat"
	_ "github.com/hexinfra/gorox/hemi/standard/dealets/tcps/echo"
	_ "github.com/hexinfra/gorox/hemi/standard/dealets/tcps/hello"
	_ "github.com/hexinfra/gorox/hemi/standard/dealets/udps/dns"
	_ "github.com/hexinfra/gorox/hemi/standard/editors/tcps/mysql"
	_ "github.com/hexinfra/gorox/hemi/standard/handlets/access"
	_ "github.com/hexinfra/gorox/hemi/standard/handlets/favicon"
	_ "github.com/hexinfra/gorox/hemi/standard/handlets/hostname"
	_ "github.com/hexinfra/gorox/hemi/standard/handlets/https"
	_ "github.com/hexinfra/gorox/hemi/standard/handlets/limit"
	_ "github.com/hexinfra/gorox/hemi/standard/handlets/referer"
	_ "github.com/hexinfra/gorox/hemi/standard/handlets/rewrite"
	_ "github.com/hexinfra/gorox/hemi/standard/handlets/sitex"
	_ "github.com/hexinfra/gorox/hemi/standard/revisers/gzip"
	_ "github.com/hexinfra/gorox/hemi/standard/revisers/head"
	_ "github.com/hexinfra/gorox/hemi/standard/revisers/replace"
	_ "github.com/hexinfra/gorox/hemi/standard/revisers/ssi"
	_ "github.com/hexinfra/gorox/hemi/standard/revisers/wrap"
	_ "github.com/hexinfra/gorox/hemi/standard/servers/echo"
	_ "github.com/hexinfra/gorox/hemi/standard/socklets/hello"
	_ "github.com/hexinfra/gorox/hemi/standard/staters/local"
	_ "github.com/hexinfra/gorox/hemi/standard/unitures/demo"
)
