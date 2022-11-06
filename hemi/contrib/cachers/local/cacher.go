// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Local cacher implementation.

package local

import (
	. "github.com/hexinfra/gorox/hemi/internal"
	"os"
	"time"
)

func init() {
	RegisterCacher("localCacher", func(name string, stage *Stage) Cacher {
		c := new(localCacher)
		c.init(name, stage)
		return c
	})
}

// localCacher
type localCacher struct {
	// Mixins
	Cacher_
	// Assocs
	stage *Stage
	// States
	cacheDir string // /path/to/dir
}

func (c *localCacher) init(name string, stage *Stage) {
	c.SetName(name)
	c.stage = stage
}

func (c *localCacher) OnConfigure() {
	// cacheDir
	c.ConfigureString("cacheDir", &c.cacheDir, func(value string) bool { return value != "" }, DataDir()+"/cachers/"+c.Name())
}
func (c *localCacher) OnPrepare() {
	// mkdirs
	if err := os.MkdirAll(c.cacheDir, 0755); err != nil {
		EnvExitln(err.Error())
	}
}
func (c *localCacher) OnShutdown() {
}

func (c *localCacher) Maintain() { // blocking
	for {
		time.Sleep(time.Second)
	}
}

func (c *localCacher) Set(key []byte, value *Centry) {
}
func (c *localCacher) Get(key []byte) (value *Centry) {
	return
}
func (c *localCacher) Del(key []byte) bool {
	return false
}
