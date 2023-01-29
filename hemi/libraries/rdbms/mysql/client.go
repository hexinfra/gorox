// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// MySQL connector implementation.

package mysql

import (
	"time"
)

type Client struct {
	DialTimeout  time.Duration
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	DSN          *DSN
}

func (c *Client) Dial() error {
	return nil
}

type DSN struct {
	Host string // 1.2.3.4
	Port string // 3306
	User string // foo
	Pass string // 123456
	Name string // dbname
	Code string // utf8mb4
}
