// Copyright (c) 2020-2024 Zhang Jingcheng <diogin@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Configuration.

package hemi

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Value is a value in config file.
type Value struct {
	kind  int16  // tokenXXX in values
	code  int16  // variable code if kind is tokenVariable
	name  string // variable name if kind is tokenVariable
	value any    // bools, integers, strings, durations, lists, and dicts
	bytes []byte // []byte of string value, to avoid the cost of []byte(s)
}

func (v *Value) IsBool() bool     { return v.kind == tokenBool }
func (v *Value) IsInteger() bool  { return v.kind == tokenInteger }
func (v *Value) IsString() bool   { return v.kind == tokenString }
func (v *Value) IsDuration() bool { return v.kind == tokenDuration }
func (v *Value) IsList() bool     { return v.kind == tokenList }
func (v *Value) IsDict() bool     { return v.kind == tokenDict }

func (v *Value) Bool() (b bool, ok bool) {
	b, ok = v.value.(bool)
	return
}
func (v *Value) Int64() (i64 int64, ok bool) {
	i64, ok = v.value.(int64)
	return
}
func toInt[T ~int | ~int8 | ~int16 | ~int32 | ~int64 | ~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64](v *Value) (i T, ok bool) {
	i64, ok := v.Int64()
	i = T(i64)
	if ok && int64(i) != i64 {
		ok = false
	}
	return
}
func (v *Value) Uint32() (u32 uint32, ok bool) { return toInt[uint32](v) }
func (v *Value) Int32() (i32 int32, ok bool)   { return toInt[int32](v) }
func (v *Value) Int16() (i16 int16, ok bool)   { return toInt[int16](v) }
func (v *Value) Int8() (i8 int8, ok bool)      { return toInt[int8](v) }
func (v *Value) Int() (i int, ok bool)         { return toInt[int](v) }
func (v *Value) String() (s string, ok bool) {
	s, ok = v.value.(string)
	return
}
func (v *Value) Bytes() (p []byte, ok bool) {
	if v.kind == tokenString {
		p, ok = v.bytes, true
	}
	return
}
func (v *Value) Duration() (d time.Duration, ok bool) {
	d, ok = v.value.(time.Duration)
	return
}
func (v *Value) List() (list []Value, ok bool) {
	list, ok = v.value.([]Value)
	return
}
func (v *Value) ListN(n int) (list []Value, ok bool) {
	list, ok = v.value.([]Value)
	if ok && n >= 0 && len(list) != n {
		ok = false
	}
	return
}
func (v *Value) StringList() (list []string, ok bool) {
	l, ok := v.value.([]Value)
	if ok {
		for _, value := range l {
			if s, isString := value.String(); isString {
				list = append(list, s)
			}
		}
	}
	return
}
func (v *Value) BytesList() (list [][]byte, ok bool) {
	l, ok := v.value.([]Value)
	if ok {
		for _, value := range l {
			if value.kind == tokenString {
				list = append(list, value.bytes)
			}
		}
	}
	return
}
func (v *Value) StringListN(n int) (list []string, ok bool) {
	l, ok := v.value.([]Value)
	if !ok {
		return
	}
	if n >= 0 && len(l) != n {
		ok = false
		return
	}
	for _, value := range l {
		if s, ok := value.String(); ok {
			list = append(list, s)
		}
	}
	return
}
func (v *Value) Dict() (dict map[string]Value, ok bool) {
	dict, ok = v.value.(map[string]Value)
	return
}
func (v *Value) StringDict() (dict map[string]string, ok bool) {
	d, ok := v.value.(map[string]Value)
	if ok {
		dict = make(map[string]string)
		for name, value := range d {
			if s, ok := value.String(); ok {
				dict[name] = s
			}
		}
	}
	return
}

func (v *Value) IsVariable() bool { return v.kind == tokenVariable }

func (v *Value) BytesVar(keeper varKeeper) []byte {
	return keeper.unsafeVariable(v.code, v.name)
}
func (v *Value) StringVar(keeper varKeeper) string {
	return string(keeper.unsafeVariable(v.code, v.name))
}

// varKeeper holdes values of variables.
type varKeeper interface {
	unsafeVariable(code int16, name string) (value []byte)
}

// config applies configuration and creates a new stage.
type config struct {
	// States
	tokens  []token // the token list
	index   int     // token index
	limit   int     // limit of token index
	counter int     // the name for components without a name
}

func (c *config) bootText(text string) (stage *Stage, err error) {
	defer func() {
		if x := recover(); x != nil {
			err = x.(error)
		}
	}()
	var l lexer
	c.tokens = l.scanText(text)
	return c.doParse()
}
func (c *config) bootFile(base string, path string) (stage *Stage, err error) {
	defer func() {
		if x := recover(); x != nil {
			err = x.(error)
		}
	}()
	var l lexer
	c.tokens = l.scanFile(base, path)
	return c.doParse()
}

func (c *config) showTokens() {
	for i := 0; i < len(c.tokens); i++ {
		token := &c.tokens[i]
		fmt.Printf("kind=%16s code=%2d line=%4d file=%s    %s\n", token.name(), token.info, token.line, token.file, token.text)
	}
}

func (c *config) current() *token { return &c.tokens[c.index] }
func (c *config) forward() *token {
	c._forwardCheckEOF()
	return &c.tokens[c.index]
}
func (c *config) currentIs(kind int16) bool { return c.tokens[c.index].kind == kind }
func (c *config) nextIs(kind int16) bool {
	if c.index == c.limit {
		return false
	}
	return c.tokens[c.index+1].kind == kind
}
func (c *config) expect(kind int16) *token {
	current := &c.tokens[c.index]
	if current.kind != kind {
		panic(fmt.Errorf("config: expect %s, but get %s=%s (in line %d)\n", tokenNames[kind], tokenNames[current.kind], current.text, current.line))
	}
	return current
}
func (c *config) forwardExpect(kind int16) *token {
	c._forwardCheckEOF()
	return c.expect(kind)
}
func (c *config) _forwardCheckEOF() {
	if c.index++; c.index == c.limit {
		panic(errors.New("config: unexpected EOF"))
	}
}

func (c *config) newName() string {
	c.counter++
	return strconv.Itoa(c.counter)
}

func (c *config) doParse() (stage *Stage, err error) {
	if current := c.current(); current.kind != tokenComponent || current.info != compStage {
		panic(errors.New("config error: root component is not stage"))
	}
	stage = createStage()
	stage.setParent(nil)
	c.parseStage(stage)
	return stage, nil
}

func (c *config) parseStage(stage *Stage) { // stage {}
	c.forwardExpect(tokenLeftBrace) // {
	for {
		current := c.forward()
		if current.kind == tokenRightBrace { // }
			return
		}
		if current.kind == tokenProperty { // .property
			c._parseAssign(current, stage)
			continue
		}
		if current.kind != tokenComponent {
			panic(fmt.Errorf("config error: unknown token %s=%s (in line %d) in stage\n", current.name(), current.text, current.line))
		}
		switch current.info {
		case compFixture:
			c.parseFixture(current, stage)
		case compAddon:
			c.parseAddon(current, stage)
		case compBackend:
			c.parseBackend(current, stage)
		case compQUICRouter:
			c.parseQUICRouter(stage)
		case compTCPSRouter:
			c.parseTCPSRouter(stage)
		case compUDPSRouter:
			c.parseUDPSRouter(stage)
		case compStater:
			c.parseStater(current, stage)
		case compCacher:
			c.parseCacher(current, stage)
		case compWebapp:
			c.parseWebapp(current, stage)
		case compService:
			c.parseService(current, stage)
		case compServer:
			c.parseServer(current, stage)
		case compCronjob:
			c.parseCronjob(current, stage)
		default:
			panic(fmt.Errorf("unknown component '%s' in stage\n", current.text))
		}
	}
}
func (c *config) parseFixture(sign *token, stage *Stage) { // xxxFixture {}
	fixtureSign := sign.text
	fixture := stage.fixture(fixtureSign)
	if fixture == nil {
		panic(errors.New("config error: unknown fixture: " + fixtureSign))
	}
	fixture.setParent(stage)
	c.forward()
	c._parseLeaf(fixture)
}
func (c *config) parseAddon(sign *token, stage *Stage) { // xxxAddon <name> {}
	parseComponent0(c, sign, stage, stage.createAddon)
}
func (c *config) parseBackend(sign *token, stage *Stage) { // xxxBackend <name> {}
	parseComponent0(c, sign, stage, stage.createBackend)
}
func (c *config) parseQUICRouter(stage *Stage) { // quicRouter <name> {}
	parseComponentR(c, stage, stage.createQUICRouter, compQUICDealet, c.parseQUICDealet, c.parseQUICCase)
}
func (c *config) parseQUICDealet(sign *token, router *QUICRouter, kase *quicCase) { // qqqDealet <name> {}, qqqDealet {}
	parseComponent1(c, sign, router, router.createDealet, kase, kase.addDealet)
}
func (c *config) parseQUICCase(router *QUICRouter) { // case <name> {}, case <name> <cond> {}, case <cond> {}, case {}
	kase := router.createCase(c.newName()) // use a temp name by default
	kase.setParent(router)
	c.forward()
	if !c.currentIs(tokenLeftBrace) { // case <name> {}, case <name> <cond> {}, case <cond> {}
		if c.currentIs(tokenString) { // case <name> {}, case <name> <cond> {}
			if caseName := c.current().text; caseName != "" {
				kase.setName(caseName) // change name
			}
			c.forward()
		}
		if !c.currentIs(tokenLeftBrace) { // case <name> <cond> {}
			c.parseCaseCond(kase)
			c.forwardExpect(tokenLeftBrace)
		}
	}
	for {
		current := c.forward()
		if current.kind == tokenRightBrace { // }
			return
		}
		if current.kind == tokenProperty { // .property
			c._parseAssign(current, kase)
			continue
		}
		if current.kind != tokenComponent {
			panic(fmt.Errorf("config error: unknown token %s=%s (in line %d) in case\n", current.name(), current.text, current.line))
		}
		switch current.info {
		case compQUICDealet:
			c.parseQUICDealet(current, router, kase)
		default:
			panic(fmt.Errorf("unknown component '%s' in quicCase\n", current.text))
		}
	}
}
func (c *config) parseTCPSRouter(stage *Stage) { // tcpsRouter <name> {}
	parseComponentR(c, stage, stage.createTCPSRouter, compTCPSDealet, c.parseTCPSDealet, c.parseTCPSCase)
}
func (c *config) parseTCPSDealet(sign *token, router *TCPSRouter, kase *tcpsCase) { // tttDealet <name> {}, tttDealet {}
	parseComponent1(c, sign, router, router.createDealet, kase, kase.addDealet)
}
func (c *config) parseTCPSCase(router *TCPSRouter) { // case <name> {}, case <name> <cond> {}, case <cond> {}, case {}
	kase := router.createCase(c.newName()) // use a temp name by default
	kase.setParent(router)
	c.forward()
	if !c.currentIs(tokenLeftBrace) { // case <name> {}, case <name> <cond> {}, case <cond> {}
		if c.currentIs(tokenString) { // case <name> {}, case <name> <cond> {}
			if caseName := c.current().text; caseName != "" {
				kase.setName(caseName) // change name
			}
			c.forward()
		}
		if !c.currentIs(tokenLeftBrace) { // case <name> <cond> {}
			c.parseCaseCond(kase)
			c.forwardExpect(tokenLeftBrace)
		}
	}
	for {
		current := c.forward()
		if current.kind == tokenRightBrace { // }
			return
		}
		if current.kind == tokenProperty { // .property
			c._parseAssign(current, kase)
			continue
		}
		if current.kind != tokenComponent {
			panic(fmt.Errorf("config error: unknown token %s=%s (in line %d) in case\n", current.name(), current.text, current.line))
		}
		switch current.info {
		case compTCPSDealet:
			c.parseTCPSDealet(current, router, kase)
		default:
			panic(fmt.Errorf("unknown component '%s' in quicCase\n", current.text))
		}
	}
}
func (c *config) parseUDPSRouter(stage *Stage) { // udpsRouter <name> {}
	parseComponentR(c, stage, stage.createUDPSRouter, compUDPSDealet, c.parseUDPSDealet, c.parseUDPSCase)
}
func (c *config) parseUDPSDealet(sign *token, router *UDPSRouter, kase *udpsCase) { // uuuDealet <name> {}, uuuDealet {}
	parseComponent1(c, sign, router, router.createDealet, kase, kase.addDealet)
}
func (c *config) parseUDPSCase(router *UDPSRouter) { // case <name> {}, case <name> <cond> {}, case <cond> {}, case {}
	kase := router.createCase(c.newName()) // use a temp name by default
	kase.setParent(router)
	c.forward()
	if !c.currentIs(tokenLeftBrace) { // case <name> {}, case <name> <cond> {}, case <cond> {}
		if c.currentIs(tokenString) { // case <name> {}, case <name> <cond> {}
			if caseName := c.current().text; caseName != "" {
				kase.setName(caseName) // change name
			}
			c.forward()
		}
		if !c.currentIs(tokenLeftBrace) { // case <name> <cond> {}
			c.parseCaseCond(kase)
			c.forwardExpect(tokenLeftBrace)
		}
	}
	for {
		current := c.forward()
		if current.kind == tokenRightBrace { // }
			return
		}
		if current.kind == tokenProperty { // .property
			c._parseAssign(current, kase)
			continue
		}
		if current.kind != tokenComponent {
			panic(fmt.Errorf("config error: unknown token %s=%s (in line %d) in case\n", current.name(), current.text, current.line))
		}
		switch current.info {
		case compUDPSDealet:
			c.parseUDPSDealet(current, router, kase)
		default:
			panic(fmt.Errorf("unknown component '%s' in quicCase\n", current.text))
		}
	}
}
func (c *config) parseCaseCond(kase interface{ setInfo(info any) }) {
	variable := c.expect(tokenVariable)
	c.forward()
	if c.currentIs(tokenFSCheck) {
		panic(errors.New("config error: fs check is not allowed in case"))
	}
	cond := caseCond{varCode: variable.info, varName: variable.text}
	compare := c.expect(tokenCompare)
	patterns := []string{}
	if current := c.forward(); current.kind == tokenString {
		patterns = append(patterns, current.text)
	} else if current.kind == tokenLeftParen { // (
		for { // each element
			current = c.forward()
			if current.kind == tokenRightParen { // )
				break
			} else if current.kind == tokenString {
				patterns = append(patterns, current.text)
			} else {
				panic(errors.New("config error: only strings are allowed in cond"))
			}
			current = c.forward()
			if current.kind == tokenRightParen { // )
				break
			} else if current.kind != tokenComma {
				panic(errors.New("config error: bad string list in cond"))
			}
		}
	} else {
		panic(errors.New("config error: bad cond pattern"))
	}
	cond.patterns = patterns
	cond.compare = compare.text
	kase.setInfo(cond)
}
func (c *config) parseStater(sign *token, stage *Stage) { // xxxStater <name> {}
	parseComponent0(c, sign, stage, stage.createStater)
}
func (c *config) parseCacher(sign *token, stage *Stage) { // xxxCacher <name> {}
	parseComponent0(c, sign, stage, stage.createCacher)
}
func (c *config) parseWebapp(sign *token, stage *Stage) { // webapp <name> {}
	webappName := c.forwardExpect(tokenString)
	webapp := stage.createWebapp(webappName.text)
	webapp.setParent(stage)
	c.forwardExpect(tokenLeftBrace) // {
	for {
		current := c.forward()
		if current.kind == tokenRightBrace { // }
			return
		}
		if current.kind == tokenProperty { // .property
			c._parseAssign(current, webapp)
			continue
		}
		if current.kind != tokenComponent {
			panic(fmt.Errorf("config error: unknown token %s=%s (in line %d) in webapp\n", current.name(), current.text, current.line))
		}
		switch current.info {
		case compHandlet:
			c.parseHandlet(current, webapp, nil)
		case compReviser:
			c.parseReviser(current, webapp, nil)
		case compSocklet:
			c.parseSocklet(current, webapp, nil)
		case compRule:
			c.parseRule(webapp)
		default:
			panic(fmt.Errorf("unknown component '%s' in webapp\n", current.text))
		}
	}
}
func (c *config) parseHandlet(sign *token, webapp *Webapp, rule *Rule) { // xxxHandlet <name> {}, xxxHandlet {}
	parseComponent2(c, sign, webapp, webapp.createHandlet, rule, rule.addHandlet)
}
func (c *config) parseReviser(sign *token, webapp *Webapp, rule *Rule) { // xxxReviser <name> {}, xxxReviser {}
	parseComponent2(c, sign, webapp, webapp.createReviser, rule, rule.addReviser)
}
func (c *config) parseSocklet(sign *token, webapp *Webapp, rule *Rule) { // xxxSocklet <name> {}, xxxSocklet {}
	parseComponent2(c, sign, webapp, webapp.createSocklet, rule, rule.addSocklet)
}
func (c *config) parseRule(webapp *Webapp) { // rule <name> {}, rule <name> <cond> {}, rule <cond> {}, rule {}
	rule := webapp.createRule(c.newName()) // use a temp name by default
	rule.setParent(webapp)
	c.forward()
	if !c.currentIs(tokenLeftBrace) { // rule <name> {}, rule <name> <cond> {}, rule <cond> {}
		if c.currentIs(tokenString) { // rule <name> {}, rule <name> <cond> {}
			if ruleName := c.current().text; ruleName != "" {
				rule.setName(ruleName) // change name
			}
			c.forward()
		}
		if !c.currentIs(tokenLeftBrace) { // rule <name> <cond> {}
			c.parseRuleCond(rule)
			c.forwardExpect(tokenLeftBrace)
		}
	}
	for {
		current := c.forward()
		if current.kind == tokenRightBrace { // }
			return
		}
		if current.kind == tokenProperty { // .property
			c._parseAssign(current, rule)
			continue
		}
		if current.kind != tokenComponent {
			panic(fmt.Errorf("config error: unknown token %s=%s (in line %d) in rule\n", current.name(), current.text, current.line))
		}
		switch current.info {
		case compHandlet:
			c.parseHandlet(current, webapp, rule)
		case compReviser:
			c.parseReviser(current, webapp, rule)
		case compSocklet:
			c.parseSocklet(current, webapp, rule)
		default:
			panic(fmt.Errorf("config error: unknown component %s=%s (in line %d) in rule\n", current.name(), current.text, current.line))
		}
	}
}
func (c *config) parseRuleCond(rule *Rule) {
	variable := c.expect(tokenVariable)
	c.forward()
	cond := ruleCond{varCode: variable.info, varName: variable.text}
	var compare *token
	if c.currentIs(tokenFSCheck) {
		if variable.text != "path" {
			panic(fmt.Errorf("config error: only path is allowed to test against file system, but got %s\n", variable.text))
		}
		compare = c.current()
	} else {
		compare = c.expect(tokenCompare)
		patterns := []string{}
		if current := c.forward(); current.kind == tokenString {
			patterns = append(patterns, current.text)
		} else if current.kind == tokenLeftParen { // (
			for { // each element
				current = c.forward()
				if current.kind == tokenRightParen { // )
					break
				} else if current.kind == tokenString {
					patterns = append(patterns, current.text)
				} else {
					panic(errors.New("config error: only strings are allowed in cond"))
				}
				current = c.forward()
				if current.kind == tokenRightParen { // )
					break
				} else if current.kind != tokenComma {
					panic(errors.New("config error: bad string list in cond"))
				}
			}
		} else {
			panic(errors.New("config error: bad cond pattern"))
		}
		cond.patterns = patterns
	}
	cond.compare = compare.text
	rule.setInfo(cond)
}
func (c *config) parseService(sign *token, stage *Stage) { // service <name> {}
	serviceName := c.forwardExpect(tokenString)
	service := stage.createService(serviceName.text)
	service.setParent(stage)
	c.forward()
	c._parseLeaf(service)
}
func (c *config) parseServer(sign *token, stage *Stage) { // xxxServer <name> {}
	parseComponent0(c, sign, stage, stage.createServer)
}
func (c *config) parseCronjob(sign *token, stage *Stage) { // xxxCronjob <name> {}
	parseComponent0(c, sign, stage, stage.createCronjob)
}

func (c *config) _parseLeaf(component Component) {
	c.expect(tokenLeftBrace) // {
	for {
		current := c.forward()
		if current.kind == tokenRightBrace { // }
			return
		}
		if current.kind == tokenProperty { // .property
			c._parseAssign(current, component)
			continue
		}
		panic(fmt.Errorf("config error: unknown token %s=%s (in line %d) in component\n", current.name(), current.text, current.line))
	}
}
func (c *config) _parseAssign(prop *token, component Component) {
	if c.nextIs(tokenLeftBrace) { // {
		panic(fmt.Errorf("config error: unknown component '%s' (in line %d)\n", prop.text, prop.line))
	}
	c.forwardExpect(tokenEqual) // =
	c.forward()
	var value Value
	c._parseValue(component, prop.text, &value)
	component.setProp(prop.text, value)
}

func (c *config) _parseValue(component Component, prop string, value *Value) {
	current := c.current()
	switch current.kind {
	case tokenBool:
		value.kind, value.value = tokenBool, current.text == "true"
	case tokenInteger:
		last := current.text[len(current.text)-1]
		if byteIsDigit(last) {
			n64, err := strconv.ParseInt(current.text, 10, 64)
			if err != nil {
				panic(fmt.Errorf("config error: bad integer %s\n", current.text))
			}
			if n64 < 0 {
				panic(errors.New("config error: negative integers are not allowed"))
			}
			value.value = n64
		} else {
			size, err := strconv.ParseInt(current.text[:len(current.text)-1], 10, 64)
			if err != nil {
				panic(fmt.Errorf("config error: bad size %s\n", current.text))
			}
			if size < 0 {
				panic(errors.New("config error: negative sizes are not allowed"))
			}
			switch current.text[len(current.text)-1] {
			case 'K':
				size *= K
			case 'M':
				size *= M
			case 'G':
				size *= G
			case 'T':
				size *= T
			}
			value.value = size
		}
		value.kind = tokenInteger
	case tokenString:
		value.kind, value.value, value.bytes = tokenString, current.text, []byte(current.text)
	case tokenDuration:
		last := len(current.text) - 1
		n, err := strconv.ParseInt(current.text[:last], 10, 64)
		if err != nil {
			panic(fmt.Errorf("config error: bad duration %s\n", current.text))
		}
		if n < 0 {
			panic(errors.New("config error: negative durations are not allowed"))
		}
		var d time.Duration
		switch current.text[last] {
		case 's':
			d = time.Duration(n) * time.Second
		case 'm':
			d = time.Duration(n) * time.Minute
		case 'h':
			d = time.Duration(n) * time.Hour
		case 'd':
			d = time.Duration(n) * 24 * time.Hour
		}
		value.kind, value.value = tokenDuration, d
	case tokenVariable: // $variable
		value.kind, value.code, value.name = tokenVariable, -1, current.text
	case tokenLeftParen: // (...)
		c._parseList(component, prop, value)
	case tokenLeftBracket: // [...]
		c._parseDict(component, prop, value)
	case tokenProperty: // .property
		if propRef := current.text; prop == "" || prop == propRef {
			panic(errors.New("config error: cannot refer to self"))
		} else if valueRef, ok := component.Find(propRef); !ok {
			panic(fmt.Errorf("config error: refer to a prop that doesn't exist in line %d\n", current.line))
		} else {
			*value = valueRef
		}
	default:
		panic(fmt.Errorf("config error: expect a value, but get token %s=%s (in line %d)\n", current.name(), current.text, current.line))
	}

	if value.kind != tokenString {
		// Currently only strings can be concatenated
		return
	}

	for {
		if !c.nextIs(tokenPlus) { // any concatenations?
			break
		}
		// Yes.
		c.forward() // +
		current = c.forward()
		var str Value
		isString := false
		if c.currentIs(tokenString) {
			isString = true
			c._parseValue(component, prop, &str)
		} else if c.currentIs(tokenProperty) {
			if propRef := current.text; prop == "" || prop == propRef {
				panic(errors.New("config error: cannot refer to self"))
			} else if valueRef, ok := component.Find(propRef); !ok {
				panic(errors.New("config error: refere to a prop that doesn't exist"))
			} else {
				str = valueRef
				if str.kind == tokenString {
					isString = true
				}
			}
		}
		if isString {
			value.value = value.value.(string) + str.value.(string)
			value.bytes = append(value.bytes, str.bytes...)
		} else {
			panic(errors.New("config error: cannot concat string with other types. token=" + c.current().text))
		}
	}
}
func (c *config) _parseList(component Component, prop string, value *Value) {
	list := []Value{}
	c.expect(tokenLeftParen) // (
	for {
		current := c.forward()
		if current.kind == tokenRightParen { // )
			break
		}
		var elem Value
		c._parseValue(component, prop, &elem)
		list = append(list, elem)
		current = c.forward()
		if current.kind == tokenRightParen { // )
			break
		} else if current.kind != tokenComma { // ,
			panic(fmt.Errorf("config error: bad list in line %d\n", current.line))
		}
	}
	value.kind, value.value = tokenList, list
}
func (c *config) _parseDict(component Component, prop string, value *Value) {
	dict := make(map[string]Value)
	c.expect(tokenLeftBracket) // [
	for {
		current := c.forward()
		if current.kind == tokenRightBracket { // ]
			break
		}
		k := c.expect(tokenString)  // k
		c.forwardExpect(tokenColon) // :
		current = c.forward()       // v
		var v Value
		c._parseValue(component, prop, &v)
		dict[k.text] = v
		current = c.forward()
		if current.kind == tokenRightBracket { // ]
			break
		} else if current.kind != tokenComma { // ,
			panic(fmt.Errorf("config error: bad dict in line %d\n", current.line))
		}
	}
	value.kind, value.value = tokenDict, dict
}

func parseComponent0[T Component](c *config, sign *token, stage *Stage, create func(sign string, name string) T) { // addon, backend, stater, cacher, server, cronjob
	name := c.forwardExpect(tokenString)
	component := create(sign.text, name.text)
	component.setParent(stage)
	c.forward()
	c._parseLeaf(component)
}
func parseComponentR[R Component, C any](c *config, stage *Stage, create func(name string) R, infoDealet int16, parseDealet func(sign *token, router R, kase *C), parseCase func(router R)) { // router
	routerName := c.forwardExpect(tokenString)
	router := create(routerName.text)
	router.setParent(stage)
	c.forwardExpect(tokenLeftBrace) // {
	for {
		current := c.forward()
		if current.kind == tokenRightBrace { // }
			return
		}
		if current.kind == tokenProperty { // .property
			c._parseAssign(current, router)
			continue
		}
		if current.kind != tokenComponent {
			panic(fmt.Errorf("config error: unknown token %s=%s (in line %d) in router\n", current.name(), current.text, current.line))
		}
		switch current.info {
		case infoDealet:
			parseDealet(current, router, nil) // not in case
		case compCase:
			parseCase(router)
		default:
			panic(fmt.Errorf("unknown component '%s' in router\n", current.text))
		}
	}
}
func parseComponent1[R Component, T Component, C any](c *config, sign *token, router R, create func(sign string, name string) T, kase *C, assign func(T)) { // dealet
	name := sign.text
	if current := c.forward(); current.kind == tokenString {
		name = current.text
		c.forward()
	} else if kase != nil { // in case
		name = c.newName()
	}
	component := create(sign.text, name)
	component.setParent(router)
	if kase != nil { // in case
		assign(component)
	}
	c._parseLeaf(component)
}
func parseComponent2[T Component](c *config, sign *token, webapp *Webapp, create func(sign string, name string) T, rule *Rule, assign func(T)) { // handlet, reviser, socklet
	name := sign.text
	if current := c.forward(); current.kind == tokenString {
		name = current.text
		c.forward()
	} else if rule != nil { // in rule
		name = c.newName()
	}
	component := create(sign.text, name)
	component.setParent(webapp)
	if rule != nil { // in rule
		assign(component)
	}
	c._parseLeaf(component)
}

// caseCond is the case condition.
type caseCond struct {
	varCode  int16    // see varCodes. set as -1 if not available
	varName  string   // used if varCode is not available
	compare  string   // ==, ^=, $=, *=, ~=, !=, !^, !$, !*, !~
	patterns []string // ...
}

// ruleCond is the rule condition.
type ruleCond struct {
	varCode  int16    // see varCodes. set as -1 if not available
	varName  string   // used if varCode is not available
	compare  string   // ==, ^=, $=, *=, ~=, !=, !^, !$, !*, !~, -f, -d, -e, -D, -E, !f, !d, !e
	patterns []string // ("GET", "POST"), ("https"), ("abc.com"), ("/hello", "/world")
}

var varCodes = map[string]int16{ // TODO
	// general conn vars for quic, tcps, and udps
	"srcHost": 0,
	"srcPort": 1,
	"udsMode": 2,
	"tlsMode": 3,

	// quic conn vars

	// tcps conn vars
	"serverName": 4,
	"nextProto":  5,

	// udps conn vars

	// web request vars
	"method":      0, // GET, POST, ...
	"scheme":      1, // http, https
	"authority":   2, // example.com, example.org:8080
	"hostname":    3, // example.com, example.org
	"colonPort":   4, // :80, :8080
	"path":        5, // /abc, /def/
	"uri":         6, // /abc?x=y, /%cc%dd?y=z&z=%ff
	"encodedPath": 7, // /abc, /%cc%dd
	"queryString": 8, // ?x=y, ?y=z&z=%ff
	"contentType": 9, // application/json
}

// lexer
type lexer struct {
	index int
	limit int
	text  string // the config text
	base  string
	file  string
}

func (l *lexer) scanText(text string) []token {
	l.text = text
	return l.scan()
}
func (l *lexer) scanFile(base string, file string) []token {
	l.text = l.load(base, file)
	l.base, l.file = base, file
	return l.scan()
}

func (l *lexer) scan() []token {
	l.index, l.limit = 0, len(l.text)
	var tokens []token
	line := int32(1)
	for l.index < l.limit {
		from := l.index
		switch b := l.text[l.index]; b {
		case ' ', '\t', '\r': // blank, ignore
			l.index++
		case '\n': // new line
			line++
			l.index++
		case '#': // shell comment
			l.nextUntil('\n')
		case '/': // line comment or stream comment
			if c := l.mustNext(); c == '/' { // line comment
				l.nextUntil('\n')
			} else if c == '*' { // stream comment
				l.index++
				for l.index < l.limit {
					if d := l.text[l.index]; d == '/' && l.text[l.index-1] == '*' {
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
			if l.index++; l.index < l.limit && l.text[l.index] == '=' { // ==
				tokens = append(tokens, token{tokenCompare, 0, line, l.file, "=="})
				l.index++
			} else { // =
				tokens = append(tokens, token{tokenEqual, 0, line, l.file, "="})
			}
		case '"', '`': // "string" or `string`
			s := l.text[l.index] // " or `
			l.index++
			l.nextUntil(s) // " or `
			l.checkEOF()
			tokens = append(tokens, token{tokenString, 0, line, l.file, l.text[from+1 : l.index]})
			l.index++
		case '<': // <includedFile>
			if l.base == "" {
				panic(errors.New("lexer: include is not allowed in text mode"))
			} else {
				l.index++
				l.nextUntil('>')
				l.checkEOF()
				file := l.text[from+1 : l.index]
				l.index++
				var ll lexer
				tokens = append(tokens, ll.scanFile(l.base, file)...)
			}
		case '%': // %constant
			l.nextAlnums()
			name := l.text[from+1 : l.index]
			var value string
			switch name {
			case "baseDir":
				value = BaseDir()
			case "logsDir":
				value = LogsDir()
			case "tmpsDir":
				value = TmpsDir()
			case "varsDir":
				value = VarsDir()
			default:
				panic(fmt.Errorf("lexer: '%%%s' is not a valid constant in line %d (%s)\n", name, line, l.file))
			}
			tokens = append(tokens, token{tokenString, 0, line, l.file, value})
		case '.': // .property
			l.nextAlnums()
			tokens = append(tokens, token{tokenProperty, 0, line, l.file, l.text[from+1 : l.index]})
		case '$': // $variable
			if !l.nextIs('=') {
				l.nextIdents()
				name := l.text[from+1 : l.index]
				code, ok := varCodes[name]
				if !ok {
					code = -1
				}
				tokens = append(tokens, token{tokenVariable, code, line, l.file, name})
				break
			}
			fallthrough // $=
		case '^', '*', '~': // ^=, *=, ~=
			if l.mustNext() != '=' {
				panic(fmt.Errorf("lexer: unknown character %c (ascii %v) in line %d (%s)\n", b, b, line, l.file))
			}
			l.index++
			tokens = append(tokens, token{tokenCompare, 0, line, l.file, l.text[from:l.index]})
		case '-': // -f, -d, -e, -D, -E
			if c := l.mustNext(); c != 'f' && c != 'd' && c != 'e' && c != 'D' && c != 'E' {
				panic(fmt.Errorf("lexer: not a valid FSCHECK in line %d (%s)\n", line, l.file))
			}
			l.index++
			tokens = append(tokens, token{tokenFSCheck, 0, line, l.file, l.text[from:l.index]})
		case '!': // !=, !^, !$, !*, !~, !f, !d, !e
			if c := l.mustNext(); c == '=' || c == '^' || c == '$' || c == '*' || c == '~' {
				tokens = append(tokens, token{tokenCompare, 0, line, l.file, l.text[from : l.index+1]})
			} else if c == 'f' || c == 'd' || c == 'e' {
				tokens = append(tokens, token{tokenFSCheck, 0, line, l.file, l.text[from : l.index+1]})
			} else {
				panic(fmt.Errorf("lexer: not a valid COMPARE or FSCHECK in line %d (%s)\n", line, l.file))
			}
			l.index++
		case '&': // &&
			if l.mustNext() != '&' {
				panic(fmt.Errorf("lexer: not a valid AND in line %d (%s)\n", line, l.file))
			}
			tokens = append(tokens, token{tokenAND, 0, line, l.file, "&&"})
			l.index++
		case '|': // ||
			if l.mustNext() != '|' {
				panic(fmt.Errorf("lexer: not a valid OR in line %d (%s)\n", line, l.file))
			}
			tokens = append(tokens, token{tokenOR, 0, line, l.file, "||"})
			l.index++
		default:
			if kind := soloKinds[b]; kind != 0 { // kind starts from 1
				tokens = append(tokens, token{kind, 0, line, l.file, soloTexts[b]})
				l.index++
			} else if byteIsAlpha(b) { // 'a-zA-Z'
				l.nextAlnums() // '0-9a-zA-Z'
				if identifier := l.text[from:l.index]; identifier == "true" || identifier == "false" {
					tokens = append(tokens, token{tokenBool, 0, line, l.file, identifier})
				} else if comp, ok := signedComps[identifier]; ok {
					tokens = append(tokens, token{tokenComponent, comp, line, l.file, identifier})
				} else {
					panic(fmt.Errorf("lexer: '%s' is not a valid component in line %d (%s)\n", identifier, line, l.file))
				}
			} else if byteIsDigit(b) { // '0-9'
				l.nextDigits()
				digits := true
				if l.index < l.limit {
					switch l.text[l.index] {
					case 's', 'm', 'h', 'd':
						digits = false
						l.index++
						tokens = append(tokens, token{tokenDuration, 0, line, l.file, l.text[from:l.index]})
					case 'K', 'M', 'G', 'T':
						digits = false
						l.index++
						tokens = append(tokens, token{tokenInteger, 0, line, l.file, l.text[from:l.index]})
					}
				}
				if digits {
					tokens = append(tokens, token{tokenInteger, 0, line, l.file, l.text[from:l.index]})
				}
			} else {
				panic(fmt.Errorf("lexer: unknown character %c (ascii %v) in line %d (%s)\n", b, b, line, l.file))
			}
		}
	}
	return tokens
}

func (l *lexer) nextUntil(b byte) {
	if i := strings.IndexByte(l.text[l.index:], b); i == -1 {
		l.index = l.limit
	} else {
		l.index += i
	}
}
func (l *lexer) nextIs(b byte) bool {
	if next := l.index + 1; next != l.limit {
		return l.text[next] == b
	}
	return false
}
func (l *lexer) mustNext() byte {
	l.index++
	l.checkEOF()
	return l.text[l.index]
}
func (l *lexer) checkEOF() {
	if l.index == l.limit {
		panic(errors.New("lexer: unexpected eof"))
	}
}
func (l *lexer) nextAlnums() {
	for l.index++; l.index < l.limit && byteIsAlnum(l.text[l.index]); l.index++ {
	}
}
func (l *lexer) nextIdents() {
	for l.index++; l.index < l.limit && byteIsIdent(l.text[l.index]); l.index++ {
	}
}
func (l *lexer) nextDigits() {
	for l.index++; l.index < l.limit && byteIsDigit(l.text[l.index]); l.index++ {
	}
}

func (l *lexer) load(base string, file string) string {
	if strings.HasPrefix(base, "http://") || strings.HasPrefix(base, "https://") {
		return l._loadURL(base, file)
	} else {
		return l._loadLFS(base, file)
	}
}
func (l *lexer) _loadLFS(base string, file string) string {
	path := file
	if file[0] != '/' {
		if base[len(base)-1] == '/' {
			path = base + file
		} else {
			path = base + "/" + file
		}
	}
	if data, err := os.ReadFile(path); err != nil {
		panic(err)
	} else {
		return string(data)
	}
}
func (l *lexer) _loadURL(base string, file string) string {
	u, err := url.Parse(base)
	if err != nil {
		panic(err)
	}
	path := u.Path + file
	if data, err := loadURL(u.Scheme, u.Host, path); err != nil {
		panic(err)
	} else {
		return data
	}
}

const ( // token list. if you change this list, change in tokenNames too.
	// Components
	tokenComponent = 1 + iota // stage, httpServer, ...
	// Properties
	tokenProperty // .listen, .maxSize, ...
	// Operators
	tokenLeftBrace    // {
	tokenRightBrace   // }
	tokenLeftBracket  // [
	tokenRightBracket // ]
	tokenLeftParen    // (
	tokenRightParen   // )
	tokenComma        // ,
	tokenColon        // :
	tokenPlus         // +
	tokenEqual        // =
	tokenCompare      // ==, ^=, $=, *=, ~=, !=, !^, !$, !*, !~
	tokenFSCheck      // -f, -d, -e, -D, -E, !f, !d, !e
	tokenAND          // &&
	tokenOR           // ||
	// Values
	tokenBool     // true, false
	tokenInteger  // 123, 16K, 256M, ...
	tokenString   // "", "abc", `def`, ...
	tokenDuration // 1s, 2m, 3h, 4d, ...
	tokenList     // lists: (...)
	tokenDict     // dicts: [...]
	tokenVariable // $method, $path, ...
)

// token is a token in config file.
type token struct { // 40 bytes
	kind int16  // tokenXXX
	info int16  // compXXX for components, or code for variables
	line int32  // at line number
	file string // file path
	text string // text literal
}

func (t token) name() string { return tokenNames[t.kind] }

var tokenNames = [...]string{ // token names. if you change this list, change in token list too.
	// Components
	tokenComponent: "component",
	// Properties
	tokenProperty: "property",
	// Operators
	tokenLeftBrace:    "leftBrace",
	tokenRightBrace:   "rightBrace",
	tokenLeftBracket:  "leftBracket",
	tokenRightBracket: "rightBracket",
	tokenLeftParen:    "leftParen",
	tokenRightParen:   "rightParen",
	tokenComma:        "comma",
	tokenColon:        "colon",
	tokenPlus:         "plus",
	tokenEqual:        "equal",
	tokenCompare:      "compare",
	tokenFSCheck:      "fsCheck",
	tokenAND:          "and",
	tokenOR:           "or",
	// Values
	tokenBool:     "bool",
	tokenInteger:  "integer",
	tokenString:   "string",
	tokenDuration: "duration",
	tokenList:     "list",
	tokenDict:     "dict",
	tokenVariable: "variable",
}

var ( // solos
	soloKinds = [256]int16{ // keep sync with soloTexts
		'{': tokenLeftBrace,
		'}': tokenRightBrace,
		'[': tokenLeftBracket,
		']': tokenRightBracket,
		'(': tokenLeftParen,
		')': tokenRightParen,
		',': tokenComma,
		':': tokenColon,
		'+': tokenPlus,
	}
	soloTexts = [...]string{ // keep sync with soloKinds
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
)
