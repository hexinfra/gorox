// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// MongoDB driver implementation.

package mongo

import (
	"time"
)

type DSN struct {
	Host string
	Port string
}

func Dial(dsn *DSN, timeout time.Duration) (*Mongo, error) {
	return nil, nil
}

type Mongo struct {
	dsn          *DSN
	writeTimeout time.Duration
	readTimeout  time.Duration
}

func (c *Mongo) Close() error {
	return nil
}
