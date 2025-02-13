// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Filesystem Hcache implementation.

package filesys

import (
	"errors"
	"os"
	"time"

	. "github.com/hexinfra/gorox/hemi"
)

func init() {
	RegisterHcache("filesysHcache", func(compName string, stage *Stage) Hcache {
		c := new(filesysHcache)
		c.onCreate(compName, stage)
		return c
	})
}

// filesysHcache caches hobjects in local file system.
type filesysHcache struct {
	// Parent
	Hcache_
	// States
	cacheDir string // /path/to/dir
}

func (c *filesysHcache) onCreate(compName string, stage *Stage) {
	c.Hcache_.OnCreate(compName, stage)
}
func (c *filesysHcache) OnShutdown() { close(c.ShutChan) } // notifies Maintain()

func (c *filesysHcache) OnConfigure() {
	// .cacheDir
	c.ConfigureString("cacheDir", &c.cacheDir, func(value string) error {
		if value != "" {
			return nil
		}
		return errors.New(".cacheDir has an invalid value")
	}, VarDir()+"/hcaches/"+c.CompName())
}
func (c *filesysHcache) OnPrepare() {
	if err := os.MkdirAll(c.cacheDir, 0755); err != nil {
		EnvExitln(err.Error())
	}
}

func (c *filesysHcache) Maintain() { // runner
	c.LoopRun(time.Second, func(now time.Time) {
		// TODO
	})
	if DebugLevel() >= 2 {
		Printf("filesysHcache=%s done\n", c.CompName())
	}
	c.Stage().DecHcache()
}

func (c *filesysHcache) Set(key []byte, hobject *Hobject) {
	// TODO
}
func (c *filesysHcache) Get(key []byte) (hobject *Hobject) {
	// TODO
	return
}
func (c *filesysHcache) Del(key []byte) bool {
	// TODO
	return false
}
