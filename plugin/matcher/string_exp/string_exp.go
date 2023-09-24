/*
 * Copyright (C) 2020-2022, IrineSistiana
 *
 * This file is part of mosdns.
 *
 * mosdns is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * mosdns is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

package string_exp

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
)

const PluginType = "string_exp"

func init() {
	sequence.MustRegMatchQuickSetup(PluginType, QuickSetup)
}

var _ sequence.Matcher = (*Matcher)(nil)

type Matcher struct {
	getStr GetStrFunc
	m      StringMatcher
}

type StringMatcher interface {
	MatchStr(s string) bool
}

type GetStrFunc func(qCtx *query_context.Context) string

func (m *Matcher) Match(_ context.Context, qCtx *query_context.Context) (bool, error) {
	return m.match(qCtx), nil
}

func (m *Matcher) match(qCtx *query_context.Context) bool {
	return m.m.MatchStr(m.getStr(qCtx))
}

func NewMatcher(f GetStrFunc, sm StringMatcher) *Matcher {
	m := &Matcher{
		getStr: f,
		m:      sm,
	}
	return m
}

// Format: "scr_string_name op [string]..."
// scr_string_name = {url_path|server_name|$env_key}
// op = {zl|eq|prefix|suffix|contains|regexp}
func QuickSetupFromStr(s string) (sequence.Matcher, error) {
	sf := strings.Fields(s)
	if len(sf) < 2 {
		return nil, errors.New("not enough args")
	}
	srcStrName := sf[0]
	op := sf[1]
	args := sf[2:]

	var sm StringMatcher
	switch op {
	case "zl":
		sm = opZl{}
	case "eq":
		m := make(map[string]struct{})
		for _, s := range args {
			m[s] = struct{}{}
		}
		sm = &opEq{m: m}
	case "regexp":
		var exps []*regexp.Regexp
		for _, s := range args {
			exp, err := regexp.Compile(s)
			if err != nil {
				return nil, fmt.Errorf("invalid reg expression, %w", err)
			}
			exps = append(exps, exp)
		}
		sm = &opRegExp{exp: exps}
	case "prefix":
		sm = &opF{s: args, f: strings.HasPrefix}
	case "suffix":
		sm = &opF{s: args, f: strings.HasSuffix}
	case "contains":
		sm = &opF{s: args, f: strings.Contains}
	default:
		return nil, fmt.Errorf("invalid operator %s", op)
	}

	var gf GetStrFunc
	if strings.HasPrefix(srcStrName, "$") {
		// Env
		envKey := strings.TrimPrefix(srcStrName, "$")
		gf = func(_ *query_context.Context) string {
			return os.Getenv(envKey)
		}
	} else {
		switch srcStrName {
		case "url_path":
			gf = getUrlPath
		case "server_name":
			gf = getServerName
		default:
			return nil, fmt.Errorf("invalid src string name %s", srcStrName)
		}
	}
	return NewMatcher(gf, sm), nil
}

// QuickSetup returns a sequence.ExecQuickSetupFunc.
func QuickSetup(_ sequence.BQ, s string) (sequence.Matcher, error) {
	return QuickSetupFromStr(s)
}

type opZl struct{}

func (op opZl) MatchStr(s string) bool {
	return len(s) == 0
}

type opEq struct {
	m map[string]struct{}
}

func (op *opEq) MatchStr(s string) bool {
	_, ok := op.m[s]
	return ok
}

type opF struct {
	s []string
	f func(s, arg string) bool
}

func (op *opF) MatchStr(s string) bool {
	for _, sub := range op.s {
		if op.f(s, sub) {
			return true
		}
	}
	return false
}

type opRegExp struct {
	exp []*regexp.Regexp
}

func (op *opRegExp) MatchStr(s string) bool {
	for _, exp := range op.exp {
		if exp.MatchString(s) {
			return true
		}
	}
	return false
}

func getUrlPath(qCtx *query_context.Context) string {
	return qCtx.ServerMeta.UrlPath
}

func getServerName(qCtx *query_context.Context) string {
	return qCtx.ServerMeta.ServerName
}
