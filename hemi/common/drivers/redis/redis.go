// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Redis driver implementation.

package redis

import (
	"time"
)

func Dial(address string, timeout time.Duration) (*Redis, error) {
	return nil, nil
}

type Redis struct {
	address string
}

func (c *Redis) Close() error {
	return nil
}
