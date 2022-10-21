// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Misc utils.

package config

func isAlpha(b byte) bool { return b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' }
func isDigit(b byte) bool { return b >= '0' && b <= '9' }
func isAlnum(b byte) bool { return isAlpha(b) || isDigit(b) }
