// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Define default reviser ranks.

package revisers

const ( // fixed: 0-5
	RankGunzip = 0
)

const ( // tunable: 6-25
	RankSSI     = 8
	RankWrap    = 10
	RankReplace = 14
)

const ( // fixed: 26-31
	RankHead = 29
	RankGzip = 31
)
