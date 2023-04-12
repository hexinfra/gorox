// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// TinyDB is a tiny RDB implementation.

package tinydb

import (
	"sync"
)

func OpenTable(path string) (*Table, error) {
	return nil, nil
}

type Table struct {
	lock sync.RWMutex
}

func (t *Table) RLock() {
}
func (t *Table) RUnlock() {
}

func (t *Table) Lock() {
}
func (t *Table) Unlock() {
}

func (t *Table) Get() (*Rows, int) {
	return nil, -1
}
func (t *Table) Set() int {
	return -1
}
func (t *Table) Add() int {
	return -1
}
func (t *Table) Del() int {
	return -1
}

func (t *Table) Close() error {
	return nil
}

type Rows struct {
}
