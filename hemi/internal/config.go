// Copyright (c) 2020-2022 Jingcheng Zhang <diogin@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Configuration.

package internal

import (
	"errors"
	"fmt"
	. "github.com/hexinfra/gorox/hemi/libraries/config"
	"strconv"
	"time"
)

func ApplyText(text string) (*Stage, error) {
	c := getConfig()
	return c.applyText(text)
}
func ApplyFile(base string, file string) (*Stage, error) {
	c := getConfig()
	return c.applyFile(base, file)
}
func getConfig() (c config) {
	constants := map[string]string{
		"baseDir": BaseDir(),
		"dataDir": DataDir(),
		"logsDir": LogsDir(),
		"tempDir": TempDir(),
	}
	c.init(constants, varCodes, signedComps)
	return
}

const ( // comp list
	compStage      = 1 + iota // stage
	compFixture               // clock, filesys, resolv, http1, http2, http3, quic, tcps, udps, unix
	compRunner                // ...
	compBackend               // HTTP1Backend, HTTP2Backend, HTTP3Backend, QUICBackend, TCPSBackend, UDPSBackend, UnixBackend
	compQUICMesher            // quicMesher
	compQUICDealet            // quicProxy, ...
	compQUICEditor            // ...
	compTCPSMesher            // tcpsMesher
	compTCPSDealet            // tcpsProxy, ...
	compTCPSEditor            // ...
	compUDPSMesher            // udpsMesher
	compUDPSDealet            // udpsProxy, ...
	compUDPSEditor            // ...
	compCase                  // case
	compStater                // localStater, redisStater, ...
	compCacher                // localCacher, redisCacher, ...
	compApp                   // app
	compHandlet               // static, ...
	compReviser               // gzipReviser, wrapReviser, ...
	compSocklet               // helloSocklet, ...
	compRule                  // rule
	compSvc                   // svc
	compServer                // httpxServer, echoServer, ...
	compCronjob               // cleanCronjob, statCronjob, ...
)

var varCodes = map[string]int16{ // predefined variables for config
	// general conn vars. keep sync with mesher_quic.go, mesher_tcps.go, and mesher_udps.go
	"srcHost": 0,
	"srcPort": 1,

	// general tcps & udps conn vars. keep sync with mesher_tcps.go and mesher_udps.go
	"transport": 2, // tcp/udp, tls/dtls

	// quic conn vars. keep sync with quicConnVariables in mesher_quic.go
	// quic stream vars. keep sync with quicConnVariables in mesher_quic.go

	// tcps conn vars. keep sync with tcpsConnVariables in mesher_tcps.go
	"serverName": 3,
	"nextProto":  4,

	// udps conn vars. keep sync with udpsConnVariables in mesher_udps.go

	// http request vars. keep sync with httpRequestVariables in server_http.go
	"method":      0, // GET, POST, ...
	"scheme":      1, // http, https
	"authority":   2, // example.com, example.org:8080
	"hostname":    3, // example.com, example.org
	"colonPort":   4, // :80, :8080
	"path":        5, // /abc, /def/
	"uri":         6, // /abc?x=y, /%cc%dd?y=z&z=%ff
	"encodedPath": 7, // /abc, /%cc%dd
	"queryString": 8, // ?x=y, ?y=z&z=%ff
	"contentType": 9, // text/html; charset=utf-8
}

var signedComps = map[string]int16{ // static comps. more dynamic comps are signed using signComp() below
	"stage":      compStage,
	"quicMesher": compQUICMesher,
	"tcpsMesher": compTCPSMesher,
	"udpsMesher": compUDPSMesher,
	"case":       compCase,
	"app":        compApp,
	"rule":       compRule,
	"svc":        compSvc,
}

func signComp(sign string, comp int16) {
	if have, signed := signedComps[sign]; signed {
		BugExitf("conflicting sign: comp=%d sign=%s\n", have, sign)
	}
	signedComps[sign] = comp
}

// config parses configuration and creates a new stage.
type config struct {
	// Mixins
	Parser_
	// States
}

func (c *config) init(constants map[string]string, varCodes map[string]int16, signedComps map[string]int16) {
	c.Parser_.Init(constants, varCodes, signedComps)
}

func (c *config) applyText(text string) (stage *Stage, err error) {
	defer func() {
		if x := recover(); x != nil {
			err = x.(error)
		}
	}()
	c.ScanText(text)
	return c.parse()
}
func (c *config) applyFile(base string, path string) (stage *Stage, err error) {
	defer func() {
		if x := recover(); x != nil {
			err = x.(error)
		}
	}()
	c.ScanFile(base, path)
	return c.parse()
}

func (c *config) parse() (stage *Stage, err error) {
	if current := c.Current(); current.Kind != TokenWord || current.Info != compStage {
		panic(errors.New("config error: root component is not stage"))
	}
	stage = createStage()
	stage.setParent(nil)
	c.parseStage(stage)
	return stage, nil
}

func (c *config) parseStage(stage *Stage) { // stage {}
	c.ForwardExpect(TokenLeftBrace) // {
	for {
		current := c.Forward()
		if current.Kind == TokenRightBrace { // }
			return
		}
		if current.Kind != TokenWord {
			panic(fmt.Errorf("config error: unknown token %s=%s (in line %d) in stage\n", current.Name(), current.Text, current.Line))
		}
		if c.NextIs(TokenEqual) { // =
			c.parseAssign(current, stage)
		} else {
			switch current.Text {
			case "fixtures":
				c.parseContainer0(compFixture, c.parseFixture, current.Text, stage)
			case "runners":
				c.parseContainer0(compRunner, c.parseRunner, current.Text, stage)
			case "backends":
				c.parseContainer0(compBackend, c.parseBackend, current.Text, stage)
			case "meshers":
				c.parseMeshers(stage)
			case "staters":
				c.parseContainer0(compStater, c.parseStater, current.Text, stage)
			case "cachers":
				c.parseContainer0(compCacher, c.parseCacher, current.Text, stage)
			case "apps":
				c.parseContainer0(compApp, c.parseApp, current.Text, stage)
			case "svcs":
				c.parseContainer0(compSvc, c.parseSvc, current.Text, stage)
			case "servers":
				c.parseContainer0(compServer, c.parseServer, current.Text, stage)
			case "cronjobs":
				c.parseContainer0(compCronjob, c.parseCronjob, current.Text, stage)
			default:
				panic(fmt.Errorf("unknown container '%s' in stage\n", current.Text))
			}
		}
	}
}
func (c *config) parseContainer0(comp int16, parseComponent func(sign Token, stage *Stage), compName string, stage *Stage) { // fixtures, runners, backends, staters, cachers, apps, svcs, servers, cronjobs {}
	c.ForwardExpect(TokenLeftBrace) // {
	for {
		current := c.Forward()
		if current.Kind == TokenRightBrace { // }
			return
		}
		if current.Kind != TokenWord || current.Info != comp {
			panic(errors.New("config error: only " + compName + " are allowed in " + compName))
		}
		parseComponent(current, stage)
	}
}

func (c *config) parseFixture(sign Token, stage *Stage) { // xxxFixture {}
	fixtureSign := sign.Text
	fixture := stage.fixture(fixtureSign)
	if fixture == nil {
		panic(errors.New("config error: unknown fixture: " + fixtureSign))
	}
	fixture.setParent(stage)
	c.Forward()
	c.parseAssigns(fixture)
}
func (c *config) parseRunner(sign Token, stage *Stage) { // xxxRunner {}
	runner := stage.createRunner(sign.Text)
	runner.setParent(stage)
	c.Forward()
	c.parseAssigns(runner)
}
func (c *config) parseBackend(sign Token, stage *Stage) { // xxxBackend <name> {}
	parseComponent0(c, sign, stage, stage.createBackend)
}
func parseComponent0[T Component](c *config, sign Token, stage *Stage, create func(sign string, name string) T) { // backend, stater, cacher, server
	name := c.ForwardExpect(TokenString)
	component := create(sign.Text, name.Text)
	component.setParent(stage)
	c.Forward()
	c.parseAssigns(component)
}
func (c *config) parseMeshers(stage *Stage) { // meshers {}
	c.ForwardExpect(TokenLeftBrace) // {
	for {
		current := c.Forward()
		if current.Kind == TokenRightBrace { // }
			return
		}
		if current.Kind != TokenWord {
			panic(errors.New("config error: only meshers are allowed in meshers"))
		}
		switch current.Info {
		case compQUICMesher:
			c.parseQUICMesher(stage)
		case compTCPSMesher:
			c.parseTCPSMesher(stage)
		case compUDPSMesher:
			c.parseUDPSMesher(stage)
		default:
			panic(errors.New("config error: only quicMesher, tcpsMesher, and udpsMesher are allowed in meshers"))
		}
	}
}
func (c *config) parseQUICMesher(stage *Stage) { // quicMesher <name> {}
	mesherName := c.ForwardExpect(TokenString)
	mesher := stage.createQUICMesher(mesherName.Text)
	mesher.setParent(stage)
	c.ForwardExpect(TokenLeftBrace) // {
	for {
		current := c.Forward()
		if current.Kind == TokenRightBrace { // }
			return
		}
		if current.Kind != TokenWord {
			panic(fmt.Errorf("config error: unknown token %s=%s (in line %d) in quicMesher\n", current.Name(), current.Text, current.Line))
		}
		if c.NextIs(TokenEqual) { // =
			c.parseAssign(current, mesher)
		} else {
			switch current.Text {
			case "dealets":
				parseContainer1(c, mesher, compQUICDealet, c.parseQUICDealet, current.Text)
			case "editors":
				parseContainer1(c, mesher, compQUICEditor, c.parseQUICEditor, current.Text)
			case "cases":
				parseCases(c, mesher, c.parseQUICCase)
			default:
				panic(fmt.Errorf("unknown container '%s' in quicMesher\n", current.Text))
			}
		}
	}
}
func (c *config) parseQUICDealet(sign Token, mesher *QUICMesher, kase *quicCase) { // qqqDealet <name> {}, qqqDealet {}
	parseComponent1(c, sign, mesher, mesher.createDealet, kase, kase.addDealet)
}
func (c *config) parseQUICEditor(sign Token, mesher *QUICMesher, kase *quicCase) { // qqqEditor <name> {}, qqqEditor {}
	parseComponent1(c, sign, mesher, mesher.createEditor, kase, kase.addEditor)
}
func (c *config) parseTCPSMesher(stage *Stage) { // tcpsMesher <name> {}
	mesherName := c.ForwardExpect(TokenString)
	mesher := stage.createTCPSMesher(mesherName.Text)
	mesher.setParent(stage)
	c.ForwardExpect(TokenLeftBrace) // {
	for {
		current := c.Forward()
		if current.Kind == TokenRightBrace { // }
			return
		}
		if current.Kind != TokenWord {
			panic(fmt.Errorf("config error: unknown token %s=%s (in line %d) in tcpsMesher\n", current.Name(), current.Text, current.Line))
		}
		if c.NextIs(TokenEqual) { // =
			c.parseAssign(current, mesher)
		} else {
			switch current.Text {
			case "dealets":
				parseContainer1(c, mesher, compTCPSDealet, c.parseTCPSDealet, current.Text)
			case "editors":
				parseContainer1(c, mesher, compTCPSEditor, c.parseTCPSEditor, current.Text)
			case "cases":
				parseCases(c, mesher, c.parseTCPSCase)
			default:
				panic(fmt.Errorf("unknown container '%s' in tcpsMesher\n", current.Text))
			}
		}
	}
}
func (c *config) parseTCPSDealet(sign Token, mesher *TCPSMesher, kase *tcpsCase) { // tttDealet <name> {}, tttDealet {}
	parseComponent1(c, sign, mesher, mesher.createDealet, kase, kase.addDealet)
}
func (c *config) parseTCPSEditor(sign Token, mesher *TCPSMesher, kase *tcpsCase) { // tttEditor <name> {}, tttEditor {}
	parseComponent1(c, sign, mesher, mesher.createEditor, kase, kase.addEditor)
}
func (c *config) parseUDPSMesher(stage *Stage) { // udpsMesher <name> {}
	mesherName := c.ForwardExpect(TokenString)
	mesher := stage.createUDPSMesher(mesherName.Text)
	mesher.setParent(stage)
	c.ForwardExpect(TokenLeftBrace) // {
	for {
		current := c.Forward()
		if current.Kind == TokenRightBrace { // }
			return
		}
		if current.Kind != TokenWord {
			panic(fmt.Errorf("config error: unknown token %s=%s (in line %d) in udpsMesher\n", current.Name(), current.Text, current.Line))
		}
		if c.NextIs(TokenEqual) { // =
			c.parseAssign(current, mesher)
		} else {
			switch current.Text {
			case "dealets":
				parseContainer1(c, mesher, compUDPSDealet, c.parseUDPSDealet, current.Text)
			case "editors":
				parseContainer1(c, mesher, compUDPSEditor, c.parseUDPSEditor, current.Text)
			case "cases":
				parseCases(c, mesher, c.parseUDPSCase)
			default:
				panic(fmt.Errorf("unknown container '%s' in udpsMesher\n", current.Text))
			}
		}
	}
}
func (c *config) parseUDPSDealet(sign Token, mesher *UDPSMesher, kase *udpsCase) { // uuuDealet <name> {}, uuuDealet {}
	parseComponent1(c, sign, mesher, mesher.createDealet, kase, kase.addDealet)
}
func (c *config) parseUDPSEditor(sign Token, mesher *UDPSMesher, kase *udpsCase) { // uuuEditor <name> {}, uuuEditor {}
	parseComponent1(c, sign, mesher, mesher.createEditor, kase, kase.addEditor)
}
func parseContainer1[M Component, C any](c *config, mesher M, comp int16, parseComponent func(sign Token, mesher M, kase *C), compName string) { // dealets, editors {}
	c.ForwardExpect(TokenLeftBrace) // {
	for {
		current := c.Forward()
		if current.Kind == TokenRightBrace { // }
			return
		}
		if current.Kind != TokenWord || current.Info != comp {
			panic(errors.New("config error: only " + compName + " are allowed in " + compName))
		}
		parseComponent(current, mesher, nil) // not in case
	}
}
func parseComponent1[M Component, T Component, C any](c *config, sign Token, mesher M, create func(sign string, name string) T, kase *C, assign func(T)) { // dealet, editor
	name := sign.Text
	if current := c.Forward(); current.Kind == TokenString {
		name = current.Text
		c.Forward()
	} else if kase != nil { // in case
		name = c.NewName()
	}
	component := create(sign.Text, name)
	component.setParent(mesher)
	if kase != nil { // in case
		assign(component)
	}
	c.parseAssigns(component)
}

func parseCases[M Component](c *config, mesher M, parseCase func(M)) { // cases {}
	c.ForwardExpect(TokenLeftBrace) // {
	for {
		current := c.Forward()
		if current.Kind == TokenRightBrace { // }
			return
		}
		if current.Kind != TokenWord || current.Info != compCase {
			panic(errors.New("config error: only cases are allowed in cases"))
		}
		parseCase(mesher)
	}
}

func (c *config) parseQUICCase(mesher *QUICMesher) { // case <name> {}, case <name> <cond> {}, case <cond> {}, case {}
	kase := mesher.createCase(c.NewName()) // use a temp name by default
	kase.setParent(mesher)
	c.Forward()
	if !c.CurrentIs(TokenLeftBrace) { // case <name> {}, case <name> <cond> {}, case <cond> {}
		if c.CurrentIs(TokenString) { // case <name> {}, case <name> <cond> {}
			if caseName := c.Current().Text; caseName != "" {
				kase.setName(caseName) // change name
			}
			c.Forward()
		}
		if !c.CurrentIs(TokenLeftBrace) { // case <name> <cond> {}
			c.parseCaseCond(kase)
			c.ForwardExpect(TokenLeftBrace)
		}
	}
	for {
		current := c.Forward()
		if current.Kind == TokenRightBrace { // }
			return
		}
		if current.Kind != TokenWord {
			panic(fmt.Errorf("config error: unknown token %s=%s (in line %d) in case\n", current.Name(), current.Text, current.Line))
		}
		if current.Info == compQUICEditor {
			c.parseQUICEditor(current, mesher, kase)
		} else {
			c.parseAssign(current, kase)
		}
	}
}
func (c *config) parseTCPSCase(mesher *TCPSMesher) { // case <name> {}, case <name> <cond> {}, case <cond> {}, case {}
	kase := mesher.createCase(c.NewName()) // use a temp name by default
	kase.setParent(mesher)
	c.Forward()
	if !c.CurrentIs(TokenLeftBrace) { // case <name> {}, case <name> <cond> {}, case <cond> {}
		if c.CurrentIs(TokenString) { // case <name> {}, case <name> <cond> {}
			if caseName := c.Current().Text; caseName != "" {
				kase.setName(caseName) // change name
			}
			c.Forward()
		}
		if !c.CurrentIs(TokenLeftBrace) { // case <name> <cond> {}
			c.parseCaseCond(kase)
			c.ForwardExpect(TokenLeftBrace)
		}
	}
	for {
		current := c.Forward()
		if current.Kind == TokenRightBrace { // }
			return
		}
		if current.Kind != TokenWord {
			panic(fmt.Errorf("config error: unknown token %s=%s (in line %d) in case\n", current.Name(), current.Text, current.Line))
		}
		if current.Info == compTCPSEditor {
			c.parseTCPSEditor(current, mesher, kase)
		} else {
			c.parseAssign(current, kase)
		}
	}
}
func (c *config) parseUDPSCase(mesher *UDPSMesher) { // case <name> {}, case <name> <cond> {}, case <cond> {}, case {}
	kase := mesher.createCase(c.NewName()) // use a temp name by default
	kase.setParent(mesher)
	c.Forward()
	if !c.CurrentIs(TokenLeftBrace) { // case <name> {}, case <name> <cond> {}, case <cond> {}
		if c.CurrentIs(TokenString) { // case <name> {}, case <name> <cond> {}
			if caseName := c.Current().Text; caseName != "" {
				kase.setName(caseName) // change name
			}
			c.Forward()
		}
		if !c.CurrentIs(TokenLeftBrace) { // case <name> <cond> {}
			c.parseCaseCond(kase)
			c.ForwardExpect(TokenLeftBrace)
		}
	}
	for {
		current := c.Forward()
		if current.Kind == TokenRightBrace { // }
			return
		}
		if current.Kind != TokenWord {
			panic(fmt.Errorf("config error: unknown token %s=%s (in line %d) in case\n", current.Name(), current.Text, current.Line))
		}
		if current.Info == compUDPSEditor {
			c.parseUDPSEditor(current, mesher, kase)
		} else {
			c.parseAssign(current, kase)
		}
	}
}
func (c *config) parseCaseCond(kase interface{ setInfo(info any) }) {
	variable := c.Expect(TokenVariable)
	c.Forward()
	cond := caseCond{varCode: variable.Info}
	var compare Token
	if c.CurrentIs(TokenFSCheck) {
		panic(errors.New("config error: fs check is not allowed in case"))
	}
	compare = c.Expect(TokenCompare)
	patterns := []string{}
	if current := c.Forward(); current.Kind == TokenString {
		patterns = append(patterns, current.Text)
	} else if current.Kind == TokenLeftParen { // (
		for { // each element
			current = c.Forward()
			if current.Kind == TokenRightParen { // )
				break
			} else if current.Kind == TokenString {
				patterns = append(patterns, current.Text)
			} else {
				panic(errors.New("config error: only strings are allowed in cond"))
			}
			current = c.Forward()
			if current.Kind == TokenRightParen { // )
				break
			} else if current.Kind != TokenComma {
				panic(errors.New("config error: bad string list in cond"))
			}
		}
	} else {
		panic(errors.New("config error: bad cond pattern"))
	}
	cond.patterns = patterns
	cond.compare = compare.Text
	kase.setInfo(cond)
}

func (c *config) parseStater(sign Token, stage *Stage) { // xxxStater <name> {}
	parseComponent0(c, sign, stage, stage.createStater)
}

func (c *config) parseCacher(sign Token, stage *Stage) { // xxxCacher <name> {}
	parseComponent0(c, sign, stage, stage.createCacher)
}

func (c *config) parseApp(sign Token, stage *Stage) { // app <name> {}
	appName := c.ForwardExpect(TokenString)
	app := stage.createApp(appName.Text)
	app.setParent(stage)
	c.ForwardExpect(TokenLeftBrace) // {
	for {
		current := c.Forward()
		if current.Kind == TokenRightBrace { // }
			return
		}
		if current.Kind != TokenWord {
			panic(fmt.Errorf("config error: unknown token %s=%s (in line %d) in app\n", current.Name(), current.Text, current.Line))
		}
		if c.NextIs(TokenEqual) { // =
			c.parseAssign(current, app)
		} else {
			switch current.Text {
			case "handlets":
				c.parseContainer2(app, compHandlet, c.parseHandlet, current.Text)
			case "revisers":
				c.parseContainer2(app, compReviser, c.parseReviser, current.Text)
			case "socklets":
				c.parseContainer2(app, compSocklet, c.parseSocklet, current.Text)
			case "rules":
				c.parseRules(app)
			default:
				panic(fmt.Errorf("unknown container '%s' in app\n", current.Text))
			}
		}
	}
}
func (c *config) parseContainer2(app *App, comp int16, parseComponent func(sign Token, app *App, rule *Rule), compName string) { // handlets, revisers, socklets {}
	c.ForwardExpect(TokenLeftBrace) // {
	for {
		current := c.Forward()
		if current.Kind == TokenRightBrace { // }
			return
		}
		if current.Kind != TokenWord || current.Info != comp {
			panic(errors.New("config error: only " + compName + " are allowed in " + compName))
		}
		parseComponent(current, app, nil) // not in rule
	}
}

func (c *config) parseHandlet(sign Token, app *App, rule *Rule) { // xxxHandlet <name> {}, xxxHandlet {}
	parseComponent2(c, sign, app, app.createHandlet, rule, rule.addHandlet)
}
func (c *config) parseReviser(sign Token, app *App, rule *Rule) { // xxxReviser <name> {}, xxxReviser {}
	parseComponent2(c, sign, app, app.createReviser, rule, rule.addReviser)
}
func (c *config) parseSocklet(sign Token, app *App, rule *Rule) { // xxxSocklet <name> {}, xxxSocklet {}
	parseComponent2(c, sign, app, app.createSocklet, rule, rule.addSocklet)
}
func parseComponent2[T Component](c *config, sign Token, app *App, create func(sign string, name string) T, rule *Rule, assign func(T)) { // handlet, reviser, socklet
	name := sign.Text
	if current := c.Forward(); current.Kind == TokenString {
		name = current.Text
		c.Forward()
	} else if rule != nil { // in rule
		name = c.NewName()
	}
	component := create(sign.Text, name)
	component.setParent(app)
	if rule != nil { // in rule
		assign(component)
	}
	c.parseAssigns(component)
}

func (c *config) parseRules(app *App) { // rules {}
	c.ForwardExpect(TokenLeftBrace) // {
	for {
		current := c.Forward()
		if current.Kind == TokenRightBrace { // }
			return
		}
		if current.Kind != TokenWord || current.Info != compRule {
			panic(errors.New("config error: only rules are allowed in rules"))
		}
		c.parseRule(app)
	}
}

func (c *config) parseRule(app *App) { // rule <name> {}, rule <name> <cond> {}, rule <cond> {}, rule {}
	rule := app.createRule(c.NewName()) // use a temp name by default
	rule.setParent(app)
	c.Forward()
	if !c.CurrentIs(TokenLeftBrace) { // rule <name> {}, rule <name> <cond> {}, rule <cond> {}
		if c.CurrentIs(TokenString) { // rule <name> {}, rule <name> <cond> {}
			if ruleName := c.Current().Text; ruleName != "" {
				rule.setName(ruleName) // change name
			}
			c.Forward()
		}
		if !c.CurrentIs(TokenLeftBrace) { // rule <name> <cond> {}
			c.parseRuleCond(rule)
			c.ForwardExpect(TokenLeftBrace)
		}
	}
	for {
		current := c.Forward()
		if current.Kind == TokenRightBrace { // }
			return
		}
		if current.Kind != TokenWord {
			panic(fmt.Errorf("config error: unknown token %s=%s (in line %d) in rule\n", current.Name(), current.Text, current.Line))
		}
		switch current.Info {
		case compHandlet:
			c.parseHandlet(current, app, rule)
		case compReviser:
			c.parseReviser(current, app, rule)
		case compSocklet:
			c.parseSocklet(current, app, rule)
		default:
			c.parseAssign(current, rule)
		}
	}
}
func (c *config) parseRuleCond(rule *Rule) {
	variable := c.Expect(TokenVariable)
	c.Forward()
	cond := ruleCond{varCode: variable.Info}
	var compare Token
	if c.CurrentIs(TokenFSCheck) {
		if variable.Text != "path" {
			panic(fmt.Errorf("config error: only path is allowed to test against file system, but got %s\n", variable.Text))
		}
		compare = c.Current()
	} else {
		compare = c.Expect(TokenCompare)
		patterns := []string{}
		if current := c.Forward(); current.Kind == TokenString {
			patterns = append(patterns, current.Text)
		} else if current.Kind == TokenLeftParen { // (
			for { // each element
				current = c.Forward()
				if current.Kind == TokenRightParen { // )
					break
				} else if current.Kind == TokenString {
					patterns = append(patterns, current.Text)
				} else {
					panic(errors.New("config error: only strings are allowed in cond"))
				}
				current = c.Forward()
				if current.Kind == TokenRightParen { // )
					break
				} else if current.Kind != TokenComma {
					panic(errors.New("config error: bad string list in cond"))
				}
			}
		} else {
			panic(errors.New("config error: bad cond pattern"))
		}
		cond.patterns = patterns
	}
	cond.compare = compare.Text
	rule.setInfo(cond)
}

func (c *config) parseSvc(sign Token, stage *Stage) { // svc <name> {}
	svcName := c.ForwardExpect(TokenString)
	svc := stage.createSvc(svcName.Text)
	svc.setParent(stage)
	c.Forward()
	c.parseAssigns(svc)
}

func (c *config) parseServer(sign Token, stage *Stage) { // xxxServer <name> {}
	parseComponent0(c, sign, stage, stage.createServer)
}
func (c *config) parseCronjob(sign Token, stage *Stage) { // xxxCronjob {}
	cronjob := stage.createCronjob(sign.Text)
	cronjob.setParent(stage)
	c.Forward()
	c.parseAssigns(cronjob)
}

func (c *config) parseAssigns(component Component) {
	c.Expect(TokenLeftBrace) // {
	for {
		switch current := c.Forward(); current.Kind {
		case TokenWord:
			c.parseAssign(current, component)
		case TokenRightBrace: // }
			return
		default:
			panic(fmt.Errorf("config error: unknown token %s=%s (in line %d) in component\n", current.Name(), current.Text, current.Line))
		}
	}
}
func (c *config) parseAssign(prop Token, component Component) {
	if c.NextIs(TokenLeftBrace) { // {
		panic(fmt.Errorf("config error: unknown component '%s' (in line %d)\n", prop.Text, prop.Line))
	}
	c.ForwardExpect(TokenEqual)
	c.Forward()
	var value Value
	c.parseValue(component, prop.Text, &value)
	component.setProp(prop.Text, value)
}

func (c *config) parseValue(component Component, prop string, value *Value) {
	current := c.Current()
	switch current.Kind {
	case TokenBool:
		*value = Value{TokenBool, current.Text == "true"}
	case TokenInteger:
		last := current.Text[len(current.Text)-1]
		if byteIsDigit(last) {
			n64, err := strconv.ParseInt(current.Text, 10, 64)
			if err != nil {
				panic(fmt.Errorf("config error: bad integer %s\n", current.Text))
			}
			if n64 < 0 {
				panic(errors.New("config error: negative integers are not allowed"))
			}
			*value = Value{TokenInteger, n64}
		} else {
			size, err := strconv.ParseInt(current.Text[:len(current.Text)-1], 10, 64)
			if err != nil {
				panic(fmt.Errorf("config error: bad size %s\n", current.Text))
			}
			if size < 0 {
				panic(errors.New("config error: negative sizes are not allowed"))
			}
			switch current.Text[len(current.Text)-1] {
			case 'K':
				size *= K
			case 'M':
				size *= M
			case 'G':
				size *= G
			case 'T':
				size *= T
			}
			*value = Value{TokenInteger, size}
		}
	case TokenString:
		*value = Value{TokenString, current.Text}
	case TokenDuration:
		last := len(current.Text) - 1
		n, err := strconv.ParseInt(current.Text[:last], 10, 64)
		if err != nil {
			panic(fmt.Errorf("config error: bad duration %s\n", current.Text))
		}
		if n < 0 {
			panic(errors.New("config error: negative durations are not allowed"))
		}
		var d time.Duration
		switch current.Text[last] {
		case 's':
			d = time.Duration(n) * time.Second
		case 'm':
			d = time.Duration(n) * time.Minute
		case 'h':
			d = time.Duration(n) * time.Hour
		case 'd':
			d = time.Duration(n) * 24 * time.Hour
		}
		*value = Value{TokenDuration, d}
	case TokenLeftParen: // (...)
		c.parseList(component, prop, value)
	case TokenLeftBracket: // [...]
		c.parseDict(component, prop, value)
	case TokenWord:
		if propRef := current.Text; prop == "" || prop == propRef {
			panic(errors.New("config error: cannot refer to self"))
		} else if valueRef, ok := component.Find(propRef); !ok {
			panic(fmt.Errorf("config error: refer to a prop that doesn't exist in line %d\n", current.Line))
		} else {
			*value = valueRef
		}
	default:
		panic(fmt.Errorf("config error: expect a value, but get token %s=%s (in line %d)\n", current.Name(), current.Text, current.Line))
	}

	if value.Kind != TokenString {
		// Currently only strings can be concatenated
		return
	}

	for {
		// Any concatenations?
		if !c.NextIs(TokenPlus) {
			// No
			break
		}
		// Yes.
		c.Forward() // +
		current = c.Forward()
		var str Value
		isString := false
		if c.CurrentIs(TokenString) {
			isString = true
			c.parseValue(component, prop, &str)
		} else if c.CurrentIs(TokenWord) {
			if propRef := current.Text; prop == "" || prop == propRef {
				panic(errors.New("config error: cannot refer to self"))
			} else if valueRef, ok := component.Find(propRef); !ok {
				panic(errors.New("config error: refere to a prop that doesn't exist"))
			} else {
				str = valueRef
				if str.Kind == TokenString {
					isString = true
				}
			}
		}
		if isString {
			value.Data = value.Data.(string) + str.Data.(string)
		} else {
			panic(errors.New("config error: cannot concat string with other types. token=" + c.Current().Text))
		}
	}
}
func (c *config) parseList(component Component, prop string, value *Value) {
	list := []Value{}
	c.Expect(TokenLeftParen) // (
	for {
		current := c.Forward()
		if current.Kind == TokenRightParen { // )
			break
		}
		var v Value
		c.parseValue(component, prop, &v)
		list = append(list, v)
		current = c.Forward()
		if current.Kind == TokenRightParen { // )
			break
		} else if current.Kind != TokenComma { // ,
			panic(fmt.Errorf("config error: bad list in line %d\n", current.Line))
		}
	}
	value.Kind = TokenList
	value.Data = list
}
func (c *config) parseDict(component Component, prop string, value *Value) {
	dict := make(map[string]Value)
	c.Expect(TokenLeftBracket) // [
	for {
		current := c.Forward()
		if current.Kind == TokenRightBracket { // ]
			break
		}
		k := c.Expect(TokenString)
		c.ForwardExpect(TokenColon) // :
		c.Forward()
		var v Value
		c.parseValue(component, prop, &v)
		dict[k.Text] = v
		current = c.Forward()
		if current.Kind == TokenRightBracket { // ]
			break
		} else if current.Kind != TokenComma { // ,
			panic(fmt.Errorf("config error: bad dict in line %d\n", current.Line))
		}
	}
	value.Kind = TokenDict
	value.Data = dict
}

// caseCond is the case condition.
type caseCond struct {
	varCode  int16    // see varCodes
	compare  string   // ==, ^=, $=, *=, ~=, !=, !^, !$, !*, !~
	patterns []string // ...
}

// ruleCond is the rule condition.
type ruleCond struct {
	varCode   int16    // see varCodes
	logicType int8     // 0:no-logic 1:and 2:or
	compType  int8     // todo, undefined. for fast comparison
	compare   string   // ==, ^=, $=, *=, ~=, !=, !^, !$, !*, !~, -f, -d, -e, -D, -E, !f, !d, !e
	patterns  []string // ("GET", "POST"), ("https"), ("abc.com"), ("/hello", "/world")
}
