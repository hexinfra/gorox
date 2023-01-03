// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// The official website in Chinese.

package zh_cn

import (
	"errors"
	"github.com/hexinfra/gorox/apps/official/zh_cn/pack"
	. "github.com/hexinfra/gorox/hemi"
	. "github.com/hexinfra/gorox/hemi/standard/handlets/sitex"
)

func init() {
	RegisterAppInit("zh_cn", func(app *App) error {
		logic := app.Handlet("logic")
		if logic == nil {
			return errors.New("no handlet named 'logic' in app config file")
		}
		sitex, ok := logic.(*Sitex) // must be Sitex handlet.
		if !ok {
			return errors.New("handlet in 'logic' rule is not Sitex handlet")
		}
		sitex.RegisterSite("front", pack.Pack{})
		return nil
	})
}
