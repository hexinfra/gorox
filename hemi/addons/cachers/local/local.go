// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Local cacher implementation.

package local

import (
	"errors"
	"os"
	"time"

	. "github.com/hexinfra/gorox/hemi"
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
	// Parent
	Cacher_
	// Assocs
	stage *Stage // current stage
	// States
	cacheDir string // /path/to/dir
}

func (c *localCacher) onCreate(name string, stage *Stage) {
	c.MakeComp(name)
	c.stage = stage
}
func (c *localCacher) OnShutdown() {
	close(c.ShutChan) // notifies Maintain()
}

func (c *localCacher) OnConfigure() {
	// cacheDir
	c.ConfigureString("cacheDir", &c.cacheDir, func(value string) error {
		if value != "" {
			return nil
		}
		return errors.New(".cacheDir has an invalid value")
	}, VarsDir()+"/cachers/"+c.Name())
}
func (c *localCacher) OnPrepare() {
	if err := os.MkdirAll(c.cacheDir, 0755); err != nil {
		EnvExitln(err.Error())
	}
}

func (c *localCacher) Maintain() { // runner
	c.Loop(time.Second, func(now time.Time) {
		// TODO
	})
	if Debug() >= 2 {
		Printf("localCacher=%s done\n", c.Name())
	}
	c.stage.DecSub()
}

func (c *localCacher) Set(key []byte, wobject *Wobject) {
	// TODO
}
func (c *localCacher) Get(key []byte) (wobject *Wobject) {
	// TODO
	return
}
func (c *localCacher) Del(key []byte) bool {
	// TODO
	return false
}
