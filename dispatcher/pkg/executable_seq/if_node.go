//     Copyright (C) 2020-2021, IrineSistiana
//
//     This file is part of mosdns.
//
//     mosdns is free software: you can redistribute it and/or modify
//     it under the terms of the GNU General Public License as published by
//     the Free Software Foundation, either version 3 of the License, or
//     (at your option) or later version.
//
//     mosdns is distributed in the hope that it will be useful,
//     but WITHOUT ANY WARRANTY; without even the implied warranty of
//     MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
//     GNU General Public License for more details.
//
//     You should have received a copy of the GNU General Public License
//     along with this program.  If not, see <https://www.gnu.org/licenses/>.

package executable_seq

import (
	"context"
	"fmt"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/pkg/utils"
	"go.uber.org/zap"
	"reflect"
	"strings"
)

type NagativeMatcher struct {
	m handler.Matcher
}

func NagateMatcher(m handler.Matcher) handler.Matcher {
	if nm, ok := m.(*NagativeMatcher); ok {
		return nm.m
	}
	return &NagativeMatcher{m: m}
}

func (n *NagativeMatcher) Match(ctx context.Context, qCtx *handler.Context) (matched bool, err error) {
	matched, err = n.m.Match(ctx, qCtx)
	if err != nil {
		return false, err
	}
	return !matched, nil
}

// IfBlockConfig is a config to build a IfBlock.
type IfBlockConfig struct {
	// Available type are:
	// 	string (a tag of registered handler.MatcherPlugin)
	// 	handler.Matcher.
	// 	a slice of interface{} and contains above type. (In this case, the logic
	// 		between multiple matchers is OR.)
	//
	If interface{} `yaml:"if"`

	// See ParseExecutableNode.
	Exec interface{} `yaml:"exec"`
}

type IfBlock struct {
	IfMatcher      handler.Matcher
	ExecutableNode handler.ExecutableChainNode
}

func ParseIfBlock(c *IfBlockConfig, logger *zap.Logger) (*IfBlock, error) {
	b := new(IfBlock)
	var err error

	if c.If != nil {
		b.IfMatcher, err = parseMatcher(c.If)
		if err != nil {
			return nil, fmt.Errorf("failed to parse if condition: %w", err)
		}
	}

	if c.Exec != nil {
		b.ExecutableNode, err = ParseExecutableNode(c.Exec, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to parse exec command: %w", err)
		}
	}

	return b, nil
}

func parseMatcher(in interface{}) (handler.Matcher, error) {
	switch v := in.(type) {
	case handler.Matcher:
		return v, nil
	case []interface{}:
		ms := make([]handler.Matcher, 0)
		for i, leaf := range v {
			m, err := parseMatcher(leaf)
			if err != nil {
				return nil, fmt.Errorf("failed to parse leaf #%d: %w", i, err)
			}
			ms = append(ms, m)
		}

		return BatchMatherOr(ms), nil
	case string:
		if strings.HasPrefix(v, "!") {
			return NagateMatcher(RefMatcherPluginNode(strings.TrimPrefix(v, "!"))), nil
		} else {
			return RefMatcherPluginNode(v), nil
		}
	default:
		return nil, fmt.Errorf("unsupported matcher type: %s", reflect.TypeOf(v).String())
	}
}

type BatchMatherOr []handler.Matcher

func (bm BatchMatherOr) Match(ctx context.Context, qCtx *handler.Context) (bool, error) {
	return utils.BoolLogic(ctx, qCtx, bm, false)
}

func (b *IfBlock) Exec(ctx context.Context, qCtx *handler.Context, next handler.ExecutableChainNode) (err error) {
	if b.IfMatcher != nil {
		ok, err := b.IfMatcher.Match(ctx, qCtx)
		if err != nil {
			return fmt.Errorf("matcher failed: %w", err)
		}
		if ok {
			if b.ExecutableNode != nil {
				err := handler.ExecChainNode(ctx, qCtx, b.ExecutableNode)
				if err != nil {
					return fmt.Errorf("exec node failed: %w", err)
				}
			}
		}
	}

	return handler.ExecChainNode(ctx, qCtx, next)
}
