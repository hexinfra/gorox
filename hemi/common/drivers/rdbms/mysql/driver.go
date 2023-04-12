// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// MySQL driver implementation.

package mysql

import (
	"time"
)

type DSN struct {
	Host string // 1.2.3.4
	Port string // 3306
	User string // foo
	Pass string // 123456
	Name string // dbname
	Code string // utf8mb4
}

func Dial(dsn *DSN, timeout time.Duration) (*MySQL, error) {
	return nil, nil
}

type MySQL struct {
	dsn          *DSN
	readTimeout  time.Duration
	writeTimeout time.Duration
}

func (c *MySQL) Close() error {
	return nil
}
