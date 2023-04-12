// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// PostgreSQL driver implementation.

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

func Dial(dsn *DSN, timeout time.Duration) (*PgSQL, error) {
	return nil, nil
}

type PgSQL struct {
	dsn          *DSN
	readTimeout  time.Duration
	writeTimeout time.Duration
}

func (c *PgSQL) Close() error {
	return nil
}
