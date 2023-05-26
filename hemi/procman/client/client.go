// Copyright (c) 2020-2023 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Control client.

package client

import (
	"fmt"
	"os"
)

func Main(action string) {
	if tell, ok := tells[action]; ok {
		tell()
	} else if call, ok := calls[action]; ok {
		call()
	} else {
		fmt.Fprintf(os.Stderr, "unknown action: %s\n", action)
	}
}
