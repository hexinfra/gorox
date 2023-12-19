// Copyright (c) 2020-2023 Feng Wei <feng19910104@gmail.com>.
// Copyright (c) 2022-2023 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE.md file.

// Referer checkers check referer header.

package referer

import (
	"bytes"
	"errors"
	"regexp"

	. "github.com/hexinfra/gorox/hemi/internal"
)

var (
	httpScheme  = []byte("http://")
	httpsScheme = []byte("https://")
)

func init() {
	RegisterHandlet("refererChecker", func(name string, stage *Stage, webapp *Webapp) Handlet {
		h := new(refererChecker)
		h.onCreate(name, stage, webapp)
		return h
	})
}

// refererChecker
type refererChecker struct {
	// Mixins
	Handlet_
	// Assocs
	stage  *Stage
	webapp *Webapp
	// States
	serverNames     [][]byte
	serverNameRules []*refererRule
	NoneReferer     bool // allow referer not to exist.
	// the 'Referer' field is present in the request header.but its value has been deleted by
	// a firewall or proxy server; such values are strings that do not start with 'http://' or 'https://'
	IsBlocked bool
}

func (h *refererChecker) onCreate(name string, stage *Stage, webapp *Webapp) {
	h.MakeComp(name)
	h.stage = stage
	h.webapp = webapp
}
func (h *refererChecker) OnShutdown() {
	h.webapp.SubDone()
}

func (h *refererChecker) OnConfigure() {
	// allow
	h.ConfigureBytesList("serverNames", &h.serverNames, func(rules [][]byte) error { return checkRule(rules) }, nil)
	// none
	h.ConfigureBool("none", &h.NoneReferer, false)
	// blocked
	h.ConfigureBool("blocked", &h.IsBlocked, false)

}
func (h *refererChecker) OnPrepare() {
	h.serverNameRules = make([]*refererRule, 0, len(h.serverNames))
	for _, serverName := range h.serverNames {
		if len(serverName) == 0 {
			continue
		}

		r := &refererRule{}
		if serverName[0] == '~' {
			r.matchType = regexpMatch
			r.pcre = regexp.MustCompile(string(serverName[1:]))
			h.serverNameRules = append(h.serverNameRules, r)
			continue
		}

		pathIndex := bytes.IndexByte(serverName, '/')
		if pathIndex == -1 {
			r.path = nil
			pathIndex = len(serverName)
		} else {
			r.path = serverName[pathIndex:]
		}

		r.hostname = serverName[:pathIndex]
		if r.hostname[0] == '*' {
			r.matchType = suffixMatch
		} else if r.hostname[len(r.hostname)-1] == '*' {
			r.matchType = prefixMatch
		} else {
			r.matchType = fullMatch
		}
		h.serverNameRules = append(h.serverNameRules, r)
	}
}

func (h *refererChecker) Handle(req Request, resp Response) (next bool) {
	var (
		hostname, path []byte
		index          = -1
		schemeLen      = 0
	)
	refererURL, ok := req.UnsafeHeader("referer")
	if !ok {
		if h.NoneReferer {
			return true
		}
		goto forbidden
	}
	if h.IsBlocked || len(h.serverNameRules) == 0 {
		return true
	}

	hostname, path, schemeLen = getHostNameAndPath(refererURL)
	if hostname == nil {
		goto forbidden
	}

	index = bytes.IndexByte(hostname, '.')
	for _, rule := range h.serverNameRules {
		if index == -1 && rule.matchType != fullMatch {
			continue
		}

		if rule.matchType == regexpMatch {
			if rule.match(refererURL[schemeLen:]) {
				return true
			}
		} else if rule.match(hostname) {
			if len(rule.path) > 0 && !bytes.HasPrefix(path, rule.path) {
				return false
			}
			return true
		}
	}

forbidden:
	resp.SetStatus(StatusForbidden)
	resp.SendBytes(nil)
	return false
}

func checkRule(rules [][]byte) error {
	for _, rule := range rules {
		if rule[0] == '~' { // regular expression
			if _, err := regexp.Compile(string(rule[1:])); err != nil {
				return err
			}
			continue
		}

		// start with http[s]:// is not allowed
		if bytes.HasPrefix(rule, httpScheme) || bytes.HasPrefix(rule, httpsScheme) {
			return errors.New(string(rule))
		}
		// not allow multiple '*', except for regular expressions.
		if bytes.Count(rule, []byte("*")) > 1 {
			return errors.New(string(rule))
		}

		pathIndex := bytes.IndexByte(rule, '/')
		if pathIndex == -1 {
			pathIndex = len(rules)
		}

		// '*' not allowed in the middle
		idx := bytes.IndexByte(rule[:pathIndex], '*')
		if idx != -1 && (idx > 0 && idx < len(rule[:pathIndex-1])) {
			return errors.New(string(rule))
		}
		if bytes.HasPrefix(rule, httpScheme) || bytes.HasPrefix(rule, httpsScheme) {
			return errors.New(string(rule))
		}
		if bytes.IndexByte(rule, '.') == -1 {
			return errors.New(string(rule))
		}
	}
	return nil
}

func getHostNameAndPath(refererURL []byte) (hostname, path []byte, schemeLen int) {
	cb := func(scheme, url []byte) bool {
		if len(url) < len(scheme) {
			return false
		}
		if scheme != nil && !bytes.HasPrefix(url[:len(scheme)], scheme) {
			return false
		}

		schemeLen = len(scheme)
		url = url[schemeLen:]
		queryIndex := bytes.IndexByte(url, '?')
		if queryIndex == -1 {
			queryIndex = len(url)
		}

		pathIndex := bytes.IndexByte(url[:queryIndex], '/')
		if pathIndex == -1 {
			pathIndex = queryIndex
		}

		portIndex := bytes.IndexByte(url[:queryIndex], ':')
		if portIndex == -1 {
			portIndex = pathIndex
		}

		hostname = url[:portIndex]
		path = url[pathIndex:queryIndex]
		return true
	}

	if cb(httpScheme, refererURL) || cb(httpsScheme, refererURL) || cb(nil, refererURL) {
		return
	}
	return
}

const (
	fullMatch = iota
	suffixMatch
	prefixMatch
	regexpMatch
)

type refererRule struct {
	matchType int
	hostname  []byte
	path      []byte
	pcre      *regexp.Regexp
}

func (r *refererRule) match(hostname []byte) bool {
	switch r.matchType {
	case fullMatch:
		return r.fullMatch(hostname)
	case suffixMatch:
		return r.suffixMatch(hostname)
	case prefixMatch:
		return r.prefixMatch(hostname)
	case regexpMatch:
		return r.regexpMatch(hostname)
	}

	return false
}

func (r *refererRule) fullMatch(hostname []byte) bool { // for example: www.bar.com
	return bytes.Equal(hostname, r.hostname)
}
func (r *refererRule) suffixMatch(hostname []byte) bool { // for example: *.bar.com
	return bytes.HasSuffix(hostname, r.hostname[1:])
}
func (r *refererRule) prefixMatch(hostname []byte) bool { // for example: www.bar.*
	return bytes.HasPrefix(hostname, r.hostname[:len(r.hostname)-1])
}
func (r *refererRule) regexpMatch(hostname []byte) bool { // for example:  ~\.bar\.
	return r.pcre.Match(hostname)
}
