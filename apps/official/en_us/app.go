// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// The official website in English.

package en_us

import (
	"errors"
	"github.com/hexinfra/gorox/apps/official/en_us/pack"
	. "github.com/hexinfra/gorox/hemi"
	. "github.com/hexinfra/gorox/hemi/standard/handlers/sitex"
)

func init() {
	RegisterAppInit("en_us", func(app *App) error {
		logic := app.Handler("logic")
		if logic == nil {
			return errors.New("no handler named 'logic' in app config file")
		}
		sitex, ok := logic.(*Sitex) // must be Sitex handler.
		if !ok {
			return errors.New("handler in 'logic' rule is not Sitex handler")
		}
		sitex.RegisterSite("front", pack.Pack{})
		return nil
	})
}
