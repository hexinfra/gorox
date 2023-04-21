// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// The official website in English.

package en_us

import (
	"errors"

	"github.com/hexinfra/gorox/hemi/gosites/apps/en_us/pack"

	. "github.com/hexinfra/gorox/hemi"
	. "github.com/hexinfra/gorox/hemi/contrib/handlets/sitex"
)

func init() {
	RegisterAppInit("en_us", func(app *App) error {
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
