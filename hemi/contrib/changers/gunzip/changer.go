// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Gunzip changers can gunzip request content.

package gunzip

import (
	. "github.com/hexinfra/gorox/hemi/contrib/changers"
	. "github.com/hexinfra/gorox/hemi/internal"
)

func init() {
	RegisterChanger("gunzipChanger", func(name string, stage *Stage, app *App) Changer {
		c := new(gunzipChanger)
		c.init(name, stage, app)
		return c
	})
}

// gunzipChanger
type gunzipChanger struct {
	// Mixins
	Changer_
	// Assocs
	stage *Stage
	app   *App
	// States
}

func (c *gunzipChanger) init(name string, stage *Stage, app *App) {
	c.SetName(name)
	c.stage = stage
	c.app = app
}

func (c *gunzipChanger) OnConfigure() {
}
func (c *gunzipChanger) OnPrepare() {
}
func (c *gunzipChanger) OnShutdown() {
}

func (c *gunzipChanger) Rank() int8 { return RankGunzip } // TODO

func (c *gunzipChanger) BeforeRecv(req Request, resp Response) { // identity
}
func (c *gunzipChanger) BeforePull(req Request, resp Response) { // chunked
}
func (c *gunzipChanger) FinishPull(req Request, resp Response) { // chunked
}

func (c *gunzipChanger) Change(req Request, resp Response, chain Chain) Chain {
	return chain
}
