// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Store implements simple tables for storing data.

package store

type Table struct {
	id     int64
	field1 int64
	field2 int64
	field3 []byte
	field4 []byte
}
