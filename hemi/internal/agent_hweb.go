// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// HWEB agent handlet passes requests to backend HWEB servers and cache responses.

package internal

// hwebAgent
type hwebAgent struct {
}

func (h *hwebAgent) Handle(req Request, resp Response) (next bool) { // reverse only
	return
}