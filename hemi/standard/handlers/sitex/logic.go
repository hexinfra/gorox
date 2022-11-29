// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Model and service.

package sitex

// Model
type Model struct {
}

func Table(name string, model any) *table {
	t := new(table)
	t.name = name
	t.model = model
	return t
}

// table
type table struct {
	name   string
	model  any
	fields []*field
}

func (t *table) Name() string {
	return t.name
}
func (t *table) Add(model any) int64 {
	return 0
}
func (t *table) Select(sql string) {
}

// field
type field struct {
	name          string
	kind          int8 // int8 int16 int32 int64 char varchar ...
	nullable      bool
	autoIncrement bool
	primaryKey    bool
}
