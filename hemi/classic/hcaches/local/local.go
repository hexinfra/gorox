// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Local HTTP hcache implementation.

package local

import (
	"errors"
	"os"
	"time"

	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterHcache("localHcache", func(compName string, stage *Stage) Hcache {
		c := new(localHcache)
		c.onCreate(compName, stage)
		return c
	})
}

// localHcache
type localHcache struct {
	// Parent
	Hcache_
	// Assocs
	stage *Stage // current stage
	// States
	cacheDir string // /path/to/dir
}

func (c *localHcache) onCreate(compName string, stage *Stage) {
	c.MakeComp(compName)
	c.stage = stage
}
func (c *localHcache) OnShutdown() {
	close(c.ShutChan) // notifies Maintain()
}

func (c *localHcache) OnConfigure() {
	// cacheDir
	c.ConfigureString("cacheDir", &c.cacheDir, func(value string) error {
		if value != "" {
			return nil
		}
		return errors.New(".cacheDir has an invalid value")
	}, VarDir()+"/hcaches/"+c.CompName())
}
func (c *localHcache) OnPrepare() {
	if err := os.MkdirAll(c.cacheDir, 0755); err != nil {
		EnvExitln(err.Error())
	}
}

func (c *localHcache) Maintain() { // runner
	c.LoopRun(time.Second, func(now time.Time) {
		// TODO
	})
	if DebugLevel() >= 2 {
		Printf("localHcache=%s done\n", c.CompName())
	}
	c.stage.DecSub() // hcache
}

func (c *localHcache) Set(key []byte, hobject *Hobject) {
	// TODO
}
func (c *localHcache) Get(key []byte) (hobject *Hobject) {
	// TODO
	return
}
func (c *localHcache) Del(key []byte) bool {
	// TODO
	return false
}
