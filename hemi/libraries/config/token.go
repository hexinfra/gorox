// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Config tokens.

package config

import (
	"fmt"
)

// Token
type Token struct { // 40 bytes
	Kind int16  // TokenXXX
	Info int16  // comp for words, or code for variables
	Line int32  // at line number
	File string // file path
	Text string // text literal
}

func (t Token) Name() string { return tokenNames[t.Kind] }
func (t Token) String() string {
	return fmt.Sprintf("kind=%16s info=%2d line=%4d file=%s    %s", t.Name(), t.Info, t.Line, t.File, t.Text)
}

const ( // token list. if you change this list, change in tokenNames too.
	// Word
	TokenWord = 1 + iota // address, hostnames, ...
	// Constants
	TokenConstant // @baseDir, @dataDir, @logsDir, @tempDir
	// Variables
	TokenVariable // %method, %path, ...
	// Operators
	TokenLeftBrace    // {
	TokenRightBrace   // }
	TokenLeftBracket  // [
	TokenRightBracket // ]
	TokenLeftParen    // (
	TokenRightParen   // )
	TokenComma        // ,
	TokenColon        // :
	TokenPlus         // +
	TokenEqual        // =
	TokenCompare      // ==, ^=, $=, *=, ~=, !=, !^, !$, !*, !~
	TokenFSCheck      // -f, -d, -e, -D, -E, !f, !d, !e
	TokenAND          // &&
	TokenOR           // ||
	// Values
	TokenBool     // true, false
	TokenInteger  // 123, 16K, 256M, ...
	TokenString   // "", "abc", `def`, ...
	TokenDuration // 1s, 2m, 3h, 4d, ...
	TokenList     // lists: (...)
	TokenDict     // dicts: [...]
)

var tokenNames = [...]string{ // token names. if you change this list, change in token list too.
	// Word
	TokenWord: "word",
	// Constants
	TokenConstant: "constant",
	// Variables
	TokenVariable: "variable",
	// Operators
	TokenLeftBrace:    "leftBrace",
	TokenRightBrace:   "rightBrace",
	TokenLeftBracket:  "leftBracket",
	TokenRightBracket: "rightBracket",
	TokenLeftParen:    "leftParen",
	TokenRightParen:   "rightParen",
	TokenComma:        "comma",
	TokenColon:        "colon",
	TokenPlus:         "plus",
	TokenEqual:        "equal",
	TokenCompare:      "compare",
	TokenFSCheck:      "fsCheck",
	TokenAND:          "and",
	TokenOR:           "or",
	// Value literals
	TokenBool:     "bool",
	TokenInteger:  "integer",
	TokenString:   "string",
	TokenDuration: "duration",
	TokenList:     "list",
	TokenDict:     "dict",
}

var soloKinds = [256]int16{ // keep sync with soloTexts
	'{': TokenLeftBrace,
	'}': TokenRightBrace,
	'[': TokenLeftBracket,
	']': TokenRightBracket,
	'(': TokenLeftParen,
	')': TokenRightParen,
	',': TokenComma,
	':': TokenColon,
	'+': TokenPlus,
}
var soloTexts = [...]string{ // keep sync with soloKinds
	'{': "{",
	'}': "}",
	'[': "[",
	']': "]",
	'(': "(",
	')': ")",
	',': ",",
	':': ":",
	'+': "+",
}
