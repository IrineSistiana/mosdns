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
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/utils"
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
	// Available type are string and handler.Matcher, or a slice of them.
	If interface{} `yaml:"if"`
	// Available type are string and handler.Matcher, or a slice of them.
	IfAnd interface{} `yaml:"if_and"`
	// See ParseExecutableNode.
	Exec interface{} `yaml:"exec"`
}

type IfBlock struct {
	IfMatcher      handler.Matcher
	IfAndMatcher   handler.Matcher
	ExecutableNode ExecutableNode
}

func ParseIfBlock(c *IfBlockConfig) (*IfBlock, error) {
	b := new(IfBlock)
	var err error

	if c.If != nil {
		b.IfMatcher, err = parseMatcher(c.If, batchLogicOr)
		if err != nil {
			return nil, fmt.Errorf("failed to parse if condition: %w", err)
		}
	}

	if c.IfAnd != nil {
		b.IfAndMatcher, err = parseMatcher(c.IfAnd, batchLogicAnd)
		if err != nil {
			return nil, fmt.Errorf("failed to parse ifAnd condition: %w", err)
		}
	}

	if c.Exec != nil {
		b.ExecutableNode, err = ParseExecutableNode(c.Exec)
		if err != nil {
			return nil, fmt.Errorf("failed to parse exec command: %w", err)
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

func (b *IfBlock) Exec(ctx context.Context, qCtx *handler.Context, logger *zap.Logger) (earlyStop bool, err error) {
	for _, matcher := range [...]handler.Matcher{b.IfMatcher, b.IfAndMatcher} {
		if matcher != nil {
			ok, err := matcher.Match(ctx, qCtx)
			if err != nil {
				return false, fmt.Errorf("matcher failed: %w", err)
			}
			if !ok {
				return false, nil
			}
		}
	}

	// exec
	if b.ExecutableNode != nil {
		earlyStop, err = b.ExecutableNode.Exec(ctx, qCtx, logger)
		if err != nil {
			return false, fmt.Errorf("exec command failed: %w", err)
		}
		if earlyStop {
			return true, nil
		}
	}

	return false, nil
}
