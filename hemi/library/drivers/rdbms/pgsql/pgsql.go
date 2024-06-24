// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Pgsql driver implementation.

package pgsql

import (
	"time"
)

type DSN struct {
	Host string // 1.2.3.4
	Port string // 5432
	User string // foo
	Pass string // 123456
	Name string // dbname
}

func Dial(dsn *DSN, timeout time.Duration) (*Pgsql, error) {
	return nil, nil
}

type Pgsql struct {
	dsn          *DSN
	writeTimeout time.Duration
	readTimeout  time.Duration
}

func (c *Pgsql) Close() error {
	return nil
}
