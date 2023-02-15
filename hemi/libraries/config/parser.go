// Copyright (c) 2020-2022 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Config parser.

package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Parser_
type Parser_ struct {
	constants   map[string]string // defined constants
	varCodes    map[string]int16  // defined var codes
	signedComps map[string]int16  // defined signed comps
	tokens      []Token           // the token list
	index       int               // token index
	limit       int               // limit of token index
	counter     int               // the name for components without a name
}

func (p *Parser_) Init(constants map[string]string, varCodes map[string]int16, signedComps map[string]int16) {
	p.constants = constants
	p.varCodes = varCodes
	p.signedComps = signedComps
}

func (p *Parser_) ScanText(text string) {
	var l lexer
	p.tokens = l.scanText(text)
	p.process()
}
func (p *Parser_) ScanFile(base string, file string) {
	var l lexer
	p.tokens = l.scanFile(base, file)
	p.process()
}
func (p *Parser_) Show() {
	for _, token := range p.tokens {
		fmt.Println(token.String())
	}
}
func (p *Parser_) process() {
	for i := 0; i < len(p.tokens); i++ {
		token := &p.tokens[i]
		switch token.Kind {
		case TokenWord: // some words are component signs
			if comp, ok := p.signedComps[token.Text]; ok {
				token.Info = comp
			}
		case TokenConstant: // evaluate constants
			if text, ok := p.constants[token.Text]; ok {
				token.Kind = TokenString
				token.Text = text
			}
		case TokenVariable: // evaluate variable codes
			if code, ok := p.varCodes[token.Text]; ok {
				token.Info = code
			}
		}
	}
}

func (p *Parser_) Current() Token            { return p.tokens[p.index] }
func (p *Parser_) CurrentIs(kind int16) bool { return p.tokens[p.index].Kind == kind }
func (p *Parser_) NextIs(kind int16) bool {
	if p.index == p.limit {
		return false
	}
	return p.tokens[p.index+1].Kind == kind
}
func (p *Parser_) Expect(kind int16) Token {
	current := p.tokens[p.index]
	if current.Kind != kind {
		panic(fmt.Errorf("parser: expect %s, but get %s=%s (in line %d)\n", tokenNames[kind], tokenNames[current.Kind], current.Text, current.Line))
	}
	return current
}
func (p *Parser_) ForwardExpect(kind int16) Token {
	if p.index++; p.index == p.limit {
		panic(errors.New("parser: unexpected EOF"))
	}
	return p.Expect(kind)
}
func (p *Parser_) Forward() Token {
	if p.index++; p.index == p.limit {
		panic(errors.New("parser: unexpected EOF"))
	}
	return p.tokens[p.index]
}
func (p *Parser_) NewName() string {
	p.counter++
	return strconv.Itoa(p.counter)
}

// lexer
type lexer struct {
	config string // the config text
	index  int
	limit  int
	base   string
	file   string
}

func (l *lexer) scanText(text string) []Token {
	l.config = text
	return l.scan()
}
func (l *lexer) scanFile(base string, file string) []Token {
	l.base = base
	l.file = file
	return l.scan()
}

func (l *lexer) scan() []Token {
	if l.file != "" {
		l.config = l.load(l.base, l.file)
	}
	l.index = 0
	l.limit = len(l.config)
	var tokens []Token
	line := int32(1)
	for l.index < l.limit {
		from := l.index
		switch b := l.config[l.index]; b {
		case ' ', '\r', '\t': // blank, ignore
			l.index++
		case '\n': // new line
			line++
			l.index++
		case '#': // shell comment
			l.nextUntil('\n')
		case '/': // line comment or stream comment
			if c := l.mustNext(); c == '/' { // line comment
				l.nextUntil('\n')
			} else if c == '*' { // shell comment
				l.index++
				for l.index < l.limit {
					if d := l.config[l.index]; d == '/' && l.config[l.index-1] == '*' {
						break
					} else {
						if d == '\n' {
							line++
						}
						l.index++
					}
				}
				l.checkEOF()
				l.index++
			} else {
				panic(fmt.Errorf("lexer: unknown character %c (ascii %v) in line %d (%s)\n", b, b, line, l.file))
			}
		case '=': // = or ==
			if l.index++; l.index < l.limit && l.config[l.index] == '=' { // ==
				tokens = append(tokens, Token{TokenCompare, 0, line, l.file, "=="})
				l.index++
			} else { // =
				tokens = append(tokens, Token{TokenEqual, 0, line, l.file, "="})
			}
		case '"', '`': // "string" or `string`
			s := l.config[l.index] // " or `
			l.index++
			l.nextUntil(s)
			l.checkEOF()
			tokens = append(tokens, Token{TokenString, 0, line, l.file, l.config[from+1 : l.index]})
			l.index++
		case '<': // <includedFile>
			if l.base == "" {
				panic(errors.New("lexer: include is not allowed in text mode"))
			} else {
				l.index++
				l.nextUntil('>')
				l.checkEOF()
				file := l.config[from+1 : l.index]
				l.index++
				var ll lexer
				tokens = append(tokens, ll.scanFile(l.base, file)...)
			}
		case '@': // @constant
			l.nextAlnums()
			tokens = append(tokens, Token{TokenConstant, 0, line, l.file, l.config[from+1 : l.index]})
		case '%': // %variable
			l.nextAlnums()
			tokens = append(tokens, Token{TokenVariable, 0, line, l.file, l.config[from+1 : l.index]})
		case '^', '$', '*', '~': // ^=, $=, *=, ~=
			if l.mustNext() != '=' {
				panic(fmt.Errorf("lexer: unknown character %c (ascii %v) in line %d (%s)\n", b, b, line, l.file))
			}
			l.index++
			tokens = append(tokens, Token{TokenCompare, 0, line, l.file, l.config[from:l.index]})
		case '-': // -f, -d, -e, -D, -E
			if c := l.mustNext(); c != 'f' && c != 'd' && c != 'e' && c != 'D' && c != 'E' {
				panic(fmt.Errorf("lexer: not a valid FSCHECK in line %d (%s)\n", line, l.file))
			}
			l.index++
			tokens = append(tokens, Token{TokenFSCheck, 0, line, l.file, l.config[from:l.index]})
		case '!': // !=, !^, !$, !*, !~, !f, !d, !e
			if c := l.mustNext(); c == '=' || c == '^' || c == '$' || c == '*' || c == '~' {
				tokens = append(tokens, Token{TokenCompare, 0, line, l.file, l.config[from : l.index+1]})
			} else if c == 'f' || c == 'd' || c == 'e' {
				tokens = append(tokens, Token{TokenFSCheck, 0, line, l.file, l.config[from : l.index+1]})
			} else {
				panic(fmt.Errorf("lexer: not a valid COMPARE or FSCHECK in line %d (%s)\n", line, l.file))
			}
			l.index++
		case '&': // &&
			if l.mustNext() != '&' {
				panic(fmt.Errorf("lexer: not a valid AND in line %d (%s)\n", line, l.file))
			}
			tokens = append(tokens, Token{TokenAND, 0, line, l.file, "&&"})
			l.index++
		case '|': // ||
			if l.mustNext() != '|' {
				panic(fmt.Errorf("lexer: not a valid OR in line %d (%s)\n", line, l.file))
			}
			tokens = append(tokens, Token{TokenOR, 0, line, l.file, "||"})
			l.index++
		default:
			if kind := soloKinds[b]; kind != 0 { // kind starts from 1
				tokens = append(tokens, Token{kind, 0, line, l.file, soloTexts[b]})
				l.index++
			} else if isAlpha(b) {
				l.nextAlnums()
				if word := l.config[from:l.index]; word == "true" || word == "false" {
					tokens = append(tokens, Token{TokenBool, 0, line, l.file, word})
				} else {
					tokens = append(tokens, Token{TokenWord, 0, line, l.file, word})
				}
			} else if isDigit(b) {
				l.nextDigits()
				digits := true
				if l.index < l.limit {
					switch l.config[l.index] {
					case 's', 'm', 'h', 'd':
						digits = false
						l.index++
						tokens = append(tokens, Token{TokenDuration, 0, line, l.file, l.config[from:l.index]})
					case 'K', 'M', 'G', 'T':
						digits = false
						l.index++
						tokens = append(tokens, Token{TokenInteger, 0, line, l.file, l.config[from:l.index]})
					}
				}
				if digits {
					tokens = append(tokens, Token{TokenInteger, 0, line, l.file, l.config[from:l.index]})
				}
			} else {
				panic(fmt.Errorf("lexer: unknown character %c (ascii %v) in line %d (%s)\n", b, b, line, l.file))
			}
		}
	}
	return tokens
}

func (l *lexer) nextUntil(b byte) {
	if i := strings.IndexByte(l.config[l.index:], b); i == -1 {
		l.index = l.limit
	} else {
		l.index += i
	}
}
func (l *lexer) mustNext() byte {
	l.index++
	l.checkEOF()
	return l.config[l.index]
}
func (l *lexer) checkEOF() {
	if l.index == l.limit {
		panic(errors.New("lexer: unexpected eof"))
	}
}
func (l *lexer) nextAlnums() {
	for l.index++; l.index < l.limit && isAlnum(l.config[l.index]); l.index++ {
	}
}
func (l *lexer) nextDigits() {
	for l.index++; l.index < l.limit && isDigit(l.config[l.index]); l.index++ {
	}
}

func (l *lexer) load(base string, file string) string {
	if true { // TODO
		return l._loadFS(base, file)
	} else {
		return l._loadURL(base, file)
	}
}
func (l *lexer) _loadFS(base string, file string) string {
	path := file
	if file[0] != '/' {
		if base[len(base)-1] == '/' {
			path = base + file
		} else {
			path = base + "/" + file
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	return string(data)
}
func (l *lexer) _loadURL(base string, file string) string {
	// TODO
	return ""
}
