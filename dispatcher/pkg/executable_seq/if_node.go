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
	"errors"
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

// IfNodeConfig is a config to build a IfNode.
type IfNodeConfig struct {
	// Available type are:
	// 	1. string (a tag of registered handler.MatcherPlugin)
	// 	2. handler.Matcher.
	// 	3. a slice of interface{} and contains above type. (In this case, the logic
	// 		between multiple matchers is OR.)
	// Must specify one condition. If both conditions are given, If takes the priority.
	If    interface{} `yaml:"if"`     // logical OR
	IfAnd interface{} `yaml:"if_and"` // logical AND

	// See ParseExecutableNode.
	Exec     interface{} `yaml:"exec"`
	ElseExec interface{} `yaml:"else_exec"`
}

// IfNode implement handler.ExecutableChainNode.
// Internal IfNode.ExecutableNode will also be linked by
// LinkPrevious and LinkNext.
type IfNode struct {
	ConditionMatcher   handler.Matcher // if ConditionMatcher is nil, IfNode is a no-op.
	ExecutableNode     handler.ExecutableChainNode
	ElseExecutableNode handler.ExecutableChainNode

	prev, next handler.ExecutableChainNode
}

func (b *IfNode) Previous() handler.ExecutableChainNode {
	return b.prev
}

func (b *IfNode) Next() handler.ExecutableChainNode {
	return b.next
}

func (b *IfNode) LinkPrevious(n handler.ExecutableChainNode) {
	b.prev = n
	if b.ExecutableNode != nil {
		b.ExecutableNode.LinkPrevious(n)
	}
	if b.ElseExecutableNode != nil {
		b.ElseExecutableNode.LinkPrevious(n)
	}
}

func (b *IfNode) LinkNext(n handler.ExecutableChainNode) {
	b.next = n
	if b.ExecutableNode != nil {
		handler.LatestNode(b.ExecutableNode).LinkNext(n)
	}
	if b.ElseExecutableNode != nil {
		handler.LatestNode(b.ElseExecutableNode).LinkNext(n)
	}
}

func ParseIfChainNode(c *IfNodeConfig, logger *zap.Logger) (*IfNode, error) {
	b := new(IfNode)

	var err error
	if c.If != nil {
		b.ConditionMatcher, err = parseMatcher(c.If, batchLogicOr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse if condition: %w", err)
		}
	} else if c.IfAnd != nil {
		b.ConditionMatcher, err = parseMatcher(c.IfAnd, batchLogicAnd)
		if err != nil {
			return nil, fmt.Errorf("failed to parse if_and condition: %w", err)
		}
	}

	if c.Exec != nil {
		b.ExecutableNode, err = ParseExecutableNode(c.Exec, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to parse exec command: %w", err)
		}
	}
	if c.ElseExec != nil {
		b.ElseExecutableNode, err = ParseExecutableNode(c.ElseExec, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to parse else_exec command: %w", err)
		}
	}

	return b, nil
}

const (
	batchLogicNoBatch int = iota
	batchLogicAnd
	batchLogicOr
)

func parseMatcher(in interface{}, batchLogic int) (handler.Matcher, error) {
	switch v := in.(type) {
	case handler.Matcher:
		return v, nil
	case []interface{}:
		if batchLogic == batchLogicNoBatch {
			return nil, errors.New("input should not be multiple matchers")
		}

		ms := make([]handler.Matcher, 0)
		for i, leaf := range v {
			m, err := parseMatcher(leaf, batchLogicNoBatch)
			if err != nil {
				return nil, fmt.Errorf("failed to parse leaf #%d: %w", i, err)
			}
			ms = append(ms, m)
		}

		if batchLogic == batchLogicAnd {
			return BatchMatherAnd(ms), nil
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

type BatchMatherAnd []handler.Matcher

func (bm BatchMatherAnd) Match(ctx context.Context, qCtx *handler.Context) (bool, error) {
	return utils.BoolLogic(ctx, qCtx, bm, true)
}

func (b *IfNode) Exec(ctx context.Context, qCtx *handler.Context, next handler.ExecutableChainNode) (err error) {
	if b.ConditionMatcher != nil {
		ok, err := b.ConditionMatcher.Match(ctx, qCtx)
		if err != nil {
			return fmt.Errorf("matcher failed: %w", err)
		}
		if ok && b.ExecutableNode != nil {
			return handler.ExecChainNode(ctx, qCtx, b.ExecutableNode)
		} else if !ok && b.ElseExecutableNode != nil {
			return handler.ExecChainNode(ctx, qCtx, b.ElseExecutableNode)
		}
	}

	return handler.ExecChainNode(ctx, qCtx, next)
}
