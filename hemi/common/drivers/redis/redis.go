// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Redis driver implementation.

package redis

type Redis struct {
	address string
}

func New(conn any) *Redis {
	redis := new(Redis)
	return redis
}

func (c *Redis) Close() error {
	return nil
}
