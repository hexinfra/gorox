// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

package board

import (
	"errors"
	. "github.com/hexinfra/gorox/cmds/gocmc/admin"
	"github.com/hexinfra/gorox/cmds/gocmc/board/pack"
	. "github.com/hexinfra/gorox/hemi"
	. "github.com/hexinfra/gorox/hemi/contrib/handlers/sitex"
)

func init() {
	RegisterHandler("boardHandler", func(name string, stage *Stage, app *App) Handler {
		h := new(boardHandler)
		h.Init(name, stage, app)
		return h
	})
	RegisterAppInit("board", func(app *App) error {
		logic := app.Handler("logic")
		if logic == nil {
			return errors.New("no handler named 'logic' in app config file")
		}
		board, ok := logic.(*boardHandler) // must be board handler.
		if !ok {
			return errors.New("handler in 'logic' rule is not board handler")
		}
		board.RegisterSite("front", pack.Pack{})
		return nil
	})
}

// boardHandler
type boardHandler struct {
	// Mixins
	Sitex
	// Assocs
	admin *AdminServer
	// States
}

func (h *boardHandler) OnPrepare() {
	h.Sitex.OnPrepare()
	h.admin = h.Stage().Server("cli").(*AdminServer)
}
