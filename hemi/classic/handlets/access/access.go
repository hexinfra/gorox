// Copyright (c) 2020-2024 Feng Wei <feng19910104@gmail.com>.
// Copyright (c) 2022-2024 HexInfra Co., Ltd.
// All rights reserved.
// Use of this source code is governed by a BSD-style license that can be found in the LICENSE file.

// Access checkers allow limiting access to certain client addresses.

package access

import (
	"bytes"
	"fmt"
	"net"
	"sort"
	"strings"

	. "github.com/hexinfra/gorox/hemi"
)

const (
	rankIP   = 16
	rankCIDR = 8
	rankAll  = 4
)

func init() {
	RegisterHandlet("accessChecker", func(name string, stage *Stage, webapp *Webapp) Handlet {
		h := new(accessChecker)
		h.onCreate(name, stage, webapp)
		return h
	})
}

// accessChecker
type accessChecker struct {
	// Parent
	Handlet_
	// Assocs
	stage  *Stage // current stage
	webapp *Webapp
	// States
	allow []string // allows access for the specified network or address.
	deny  []string // denies access for the specified network or address.

	allowRules []*ipRule
	denyRules  []*ipRule
}

func (h *accessChecker) onCreate(name string, stage *Stage, webapp *Webapp) {
	h.MakeComp(name)
	h.stage = stage
	h.webapp = webapp
}
func (h *accessChecker) OnShutdown() {
	h.webapp.DecSub() // handlet
}

func (h *accessChecker) OnConfigure() {
	// allow
	h.ConfigureStringList("allow", &h.allow, func(rules []string) error { return checkRule(rules) }, []string{"all"})

	// deny
	h.ConfigureStringList("deny", &h.deny, func(rules []string) error {
		if err := checkRule(rules); err != nil {
			return err
		}
		return checkRuleConflict(h.allow, rules)
	}, nil)
}
func (h *accessChecker) OnPrepare() {
	h.allowRules = h.parseRule(h.allow)
	h.denyRules = h.parseRule(h.deny)

	// sort by priority
	sort.Sort(ipRules(h.allowRules))
	sort.Sort(ipRules(h.denyRules))
}

// priority: ip > ip/24 > ip/16 > all
func (h *accessChecker) Handle(req Request, resp Response) (handled bool) {
	if len(h.denyRules) == 0 {
		return false
	}
	var (
		ip        = addressToIP(req.RemoteAddr().String())
		allowRank = -1
		denyRank  = -1
		allowMask []byte
		denyMask  []byte
	)

	for _, rule := range h.allowRules {
		if ip.Equal(rule.ip) {
			return false
		}
		if rule.cidr != nil && rule.rank > allowRank && rule.cidr.Contains(ip) {
			allowRank = rule.rank
			allowMask = rule.cidr.Mask
			break
		}
		if rule.all {
			allowRank = rule.rank
			break
		}
	}

	for _, rule := range h.denyRules {
		if ip.Equal(rule.ip) {
			goto forbidden
		}
		if rule.cidr != nil && rule.rank > denyRank && rule.cidr.Contains(ip) {
			denyRank = rule.rank
			denyMask = rule.cidr.Mask
			break
		}
		if rule.all {
			denyRank = rule.rank
			break
		}
	}

	if allowRank > denyRank {
		return false
	}
	if allowRank == denyRank && bytes.Compare(allowMask, denyMask) == 1 {
		return false
	}

forbidden:
	resp.SetStatus(StatusForbidden)
	resp.SendBytes(nil)
	return true
}

func (h *accessChecker) parseRule(rules []string) []*ipRule {
	p := make([]*ipRule, 0, len(rules))
	for _, rule := range rules {
		if rule == "all" {
			p = append(p, &ipRule{all: true, rank: rankAll})
		} else if ip := net.ParseIP(rule); ip != nil {
			p = append(p, &ipRule{ip: ip, rank: rankIP})
		} else if _, ipnet, err := net.ParseCIDR(rule); err == nil {
			p = append(p, &ipRule{cidr: ipnet, rank: rankCIDR})
		} else {
			//h.stage.Logf("accessChecker illegal ip rule: %v", rule)
		}
	}
	return p
}

type ipRule struct {
	ip   net.IP
	cidr *net.IPNet
	all  bool
	rank int
}

func (r *ipRule) String() string {
	if r.all {
		return "all"
	} else if r.ip != nil {
		return r.ip.String()
	} else if r.cidr != nil {
		return r.cidr.String()
	}

	return "unkown"
}

type ipRules []*ipRule

func (s ipRules) Len() int      { return len(s) }
func (s ipRules) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s ipRules) Less(i, j int) bool {
	if s[i].rank > s[j].rank {
		return true
	}
	if s[i].rank == s[j].rank && s[i].rank == rankCIDR &&
		bytes.Compare(s[i].cidr.Mask, s[j].cidr.Mask) == 1 {
		return true
	}
	return false
}

func checkRule(rules []string) error {
	for _, rule := range rules {
		if rule == "all" {
			continue
		}
		if ip := net.ParseIP(rule); ip != nil {
			continue
		}
		if _, _, err := net.ParseCIDR(rule); err != nil {
			return err
		}
	}
	return nil
}

func checkRuleConflict(rules1, rules2 []string) error {
	for _, rule1 := range rules1 {
		for _, rule2 := range rules2 {
			if rule1 == rule2 {
				return fmt.Errorf("%s in both .allow and .deny", rule1)
			}
		}
	}
	return nil
}

func addressToIP(address string) net.IP {
	if address[0] == '[' { // ipv6
		lastIndex := strings.IndexByte(address, ']')
		return net.ParseIP(address[1:lastIndex])
	}

	// ipv4
	lastIndex := strings.IndexByte(address, ':')
	return net.ParseIP(address[:lastIndex])
}
