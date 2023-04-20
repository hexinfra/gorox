// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Redis backend implementation.

package redis

import (
	_ "github.com/hexinfra/gorox/hemi/common/drivers/redis"
	. "github.com/hexinfra/gorox/hemi/internal"
)

// RedisBackend is a group of redis nodes.
type RedisBackend struct {
	// Mixins
	Backend_[*redisNode]
}

// redisNode is a node in RedisBackend.
type redisNode struct {
	// Mixins
	Node_
	// Assocs
	backend *RedisBackend
}

func (n *redisNode) Maintain() { // goroutine
}

// redisConn is a connection to redisNode.
type redisConn struct {
}