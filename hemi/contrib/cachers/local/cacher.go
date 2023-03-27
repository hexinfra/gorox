// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
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
		c.onCreate(name, stage)
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

func (c *localCacher) onCreate(name string, stage *Stage) {
	c.MakeComp(name)
	c.stage = stage
}
func (c *localCacher) OnShutdown() {
	close(c.Shut)
}

func (c *localCacher) OnConfigure() {
	// cacheDir
	c.ConfigureString("cacheDir", &c.cacheDir, func(value string) bool { return value != "" }, VarsDir()+"/cachers/"+c.Name())
}
func (c *localCacher) OnPrepare() {
	// mkdirs
	if err := os.MkdirAll(c.cacheDir, 0755); err != nil {
		EnvExitln(err.Error())
	}
}

func (c *localCacher) Maintain() { // goroutine
	Loop(time.Second, c.Shut, func(now time.Time) {
		// TODO
	})
	if IsDebug(2) {
		Debugf("localCacher=%s done\n", c.Name())
	}
	c.stage.SubDone()
}

func (c *localCacher) Set(key []byte, hobject *Hobject) {
	// TODO
}
func (c *localCacher) Get(key []byte) (hobject *Hobject) {
	// TODO
	return
}
func (c *localCacher) Del(key []byte) bool {
	// TODO
	return false
}
