// Copyright (c) 2020-2025 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// View.

package sitex

// value
type value struct {
	token int
	value any
}

// view
type view struct {
	parent   *view
	children []*view
	// template ast
}

func (v *view) render(args map[string]value) string {
	return ""
}
func (v *view) compile() error {
	return nil
}

var (
	htmlLL = []byte("{{")
	htmlRR = []byte("}}")
)
