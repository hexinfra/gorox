// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HRPC protocol elements, incoming message and outgoing message implementation.

// HRPC is a request/response RPC protocol.
// HRPC is under design, maybe we'll build it upon QUIC or Homa (https://homa-transport.atlassian.net/wiki/spaces/HOMA/overview).

package internal

// hrpcIn_
type hrpcIn_ = rpcIn_

// hrpcOut_
type hrpcOut_ = rpcOut_
