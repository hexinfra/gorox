// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

package webui

import (
	"errors"

	"github.com/hexinfra/gorox/cmds/myrox/apps/webui/pack"

	. "github.com/hexinfra/gorox/cmds/myrox/srvs/rocks"
	. "github.com/hexinfra/gorox/hemi"
	. "github.com/hexinfra/gorox/hemi/contrib/handlets/sitex"
)

func init() {
	RegisterAppInit("webui", func(app *App) error {
		logic := app.Handlet("logic")
		if logic == nil {
			return errors.New("no handlet named 'logic' in app config file")
		}
		webui, ok := logic.(*webuiHandlet) // must be webui handlet.
		if !ok {
			return errors.New("handlet in 'logic' rule is not webui handlet")
		}
		webui.RegisterSite("front", pack.Pack{})
		return nil
	})
}

func init() {
	RegisterHandlet("webuiHandlet", func(name string, stage *Stage, app *App) Handlet {
		h := new(webuiHandlet)
		h.OnCreate(name, stage, app)
		return h
	})
}

// webuiHandlet
type webuiHandlet struct {
	// Mixins
	Sitex
	// Assocs
	rocks *RocksServer
	// States
}

func (h *webuiHandlet) OnPrepare() {
	h.Sitex.OnPrepare()
	h.rocks = h.Stage().Server("rocks").(*RocksServer)
}
