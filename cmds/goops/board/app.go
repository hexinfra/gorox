// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

package board

import (
	"errors"
	"github.com/hexinfra/gorox/cmds/goops/board/pack"
	. "github.com/hexinfra/gorox/cmds/goops/rocks"
	. "github.com/hexinfra/gorox/hemi"
	. "github.com/hexinfra/gorox/hemi/contrib/handlets/sitex"
)

func init() {
	RegisterAppInit("board", func(app *App) error {
		logic := app.Handlet("logic")
		if logic == nil {
			return errors.New("no handlet named 'logic' in app config file")
		}
		board, ok := logic.(*boardHandlet) // must be board handlet.
		if !ok {
			return errors.New("handlet in 'logic' rule is not board handlet")
		}
		board.RegisterSite("front", pack.Pack{})
		return nil
	})
}

func init() {
	RegisterHandlet("boardHandlet", func(name string, stage *Stage, app *App) Handlet {
		h := new(boardHandlet)
		h.OnCreate(name, stage, app)
		return h
	})
}

// boardHandlet
type boardHandlet struct {
	// Mixins
	Sitex
	// Assocs
	rocks *RocksServer
	// States
}

func (h *boardHandlet) OnPrepare() {
	h.Sitex.OnPrepare()
	h.rocks = h.Stage().Server("cli").(*RocksServer)
}
