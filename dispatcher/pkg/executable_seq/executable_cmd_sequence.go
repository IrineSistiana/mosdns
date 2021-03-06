//     Copyright (C) 2020-2021, IrineSistiana
//
//     This file is part of mosdns.
//
//     mosdns is free software: you can redistribute it and/or modify
//     it under the terms of the GNU General Public License as published by
//     the Free Software Foundation, either version 3 of the License, or
//     (at your option) any later version.
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

// EarlyStop is a noop ExecutableCmd that returns earlyStop == true.
var EarlyStop earlyStop

type earlyStop struct{}

func (e earlyStop) ExecCmd(ctx context.Context, qCtx *handler.Context, logger *zap.Logger) (earlyStop bool, err error) {
	return true, nil
}

// RefESExecutablePlugin is a handler.ESExecutablePlugin reference tag.
type RefESExecutablePlugin string

func (ref RefESExecutablePlugin) ExecES(ctx context.Context, qCtx *handler.Context) (earlyStop bool, err error) {
	p, err := handler.GetPlugin(string(ref))
	if err != nil {
		return false, err
	}

	return p.ExecES(ctx, qCtx)
}

func (ref RefESExecutablePlugin) ExecCmd(ctx context.Context, qCtx *handler.Context, logger *zap.Logger) (earlyStop bool, err error) {
	earlyStop, err = ref.ExecES(ctx, qCtx)
	return earlyStop, err
}

// RefMatcherPlugin is a handler.MatcherPlugin reference tag.
type RefMatcherPlugin string

func (ref RefMatcherPlugin) Match(ctx context.Context, qCtx *handler.Context) (matched bool, err error) {
	p, err := handler.GetPlugin(string(ref))
	if err != nil {
		return false, err
	}
	return p.Match(ctx, qCtx)
}

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
	// See ParseExecutableCmd.
	Exec interface{} `yaml:"exec"`
}

type IfBlock struct {
	IfMatcher     handler.Matcher
	IfAndMatcher  handler.Matcher
	ExecutableCmd ExecutableCmd
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
		b.ExecutableCmd, err = ParseExecutableCmd(c.Exec)
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
			return NagateMatcher(RefMatcherPlugin(strings.TrimPrefix(v, "!"))), nil
		} else {
			return RefMatcherPlugin(v), nil
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

func (b *IfBlock) ExecCmd(ctx context.Context, qCtx *handler.Context, logger *zap.Logger) (earlyStop bool, err error) {
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
	if b.ExecutableCmd != nil {
		earlyStop, err = b.ExecutableCmd.ExecCmd(ctx, qCtx, logger)
		if err != nil {
			return false, fmt.Errorf("exec command failed: %w", err)
		}
		if earlyStop {
			return true, nil
		}
	}

	return false, nil
}

type ExecutableCmdSequence []ExecutableCmd

func ParseExecutableCmdSequence(in []interface{}) (ExecutableCmdSequence, error) {
	ecs := make([]ExecutableCmd, 0, len(in))
	for i, v := range in {
		ec, err := ParseExecutableCmd(v)
		if err != nil {
			return nil, fmt.Errorf("invalid cmd #%d: %w", i, err)
		}
		ecs = append(ecs, ec)
	}
	return ecs, nil
}

// ExecCmd executes the sequence.
func (es ExecutableCmdSequence) ExecCmd(ctx context.Context, qCtx *handler.Context, logger *zap.Logger) (earlyStop bool, err error) {
	for _, cmd := range es {
		earlyStop, err = cmd.ExecCmd(ctx, qCtx, logger)
		if err != nil {
			return false, err
		}
		if earlyStop {
			return true, nil
		}
	}

	return false, nil
}

// ParseExecutableCmd parses in into a ExecutableCmd.
// in can be: a handler.Executable or a handler.ESExecutable or a string
// or a map[string]interface{}, which can be parsed to FallbackConfig, ParallelECSConfig or IfBlockConfig,
// or a slice of all of above.
func ParseExecutableCmd(in interface{}) (ExecutableCmd, error) {
	switch v := in.(type) {
	case ExecutableCmd:
		return v, nil
	case []interface{}:
		return ParseExecutableCmdSequence(v)
	case handler.Executable:
		return &warpExec{e: v}, nil
	case handler.ESExecutable:
		return &warpESExec{e: v}, nil
	case string:
		return RefESExecutablePlugin(v), nil
	case map[string]interface{}:
		switch {
		case hasKey(v, "if") || hasKey(v, "if_and"): // if block
			ec, err := parseIfBlock(v)
			if err != nil {
				return nil, fmt.Errorf("invalid if section: %w", err)
			}
			return ec, nil
		case hasKey(v, "parallel"): // parallel
			ec, err := parseParallelECS(v)
			if err != nil {
				return nil, fmt.Errorf("invalid parallel section: %w", err)
			}
			return ec, nil
		case hasKey(v, "primary") || hasKey(v, "secondary"): // fallback
			ec, err := parseFallbackECS(v)
			if err != nil {
				return nil, fmt.Errorf("invalid fallback section: %w", err)
			}
			return ec, nil
		default:
			return nil, errors.New("unknown section")
		}
	default:
		return nil, fmt.Errorf("unexpected type: %s", reflect.TypeOf(in).String())
	}
}

func parseIfBlock(m map[string]interface{}) (ec ExecutableCmd, err error) {
	conf := new(IfBlockConfig)
	err = handler.WeakDecode(m, conf)
	if err != nil {
		return nil, err
	}

	return ParseIfBlock(conf)
}

func parseParallelECS(m map[string]interface{}) (ec ExecutableCmd, err error) {
	conf := new(ParallelECSConfig)
	err = handler.WeakDecode(m, conf)
	if err != nil {
		return nil, err
	}
	return ParseParallelECS(conf)
}

func parseFallbackECS(m map[string]interface{}) (ec ExecutableCmd, err error) {
	conf := new(FallbackConfig)
	err = handler.WeakDecode(m, conf)
	if err != nil {
		return nil, err
	}
	return ParseFallbackECS(conf)
}

func hasKey(m map[string]interface{}, key string) bool {
	_, ok := m[key]
	return ok
}

type warpExec struct {
	e handler.Executable
}

func (w *warpExec) ExecCmd(ctx context.Context, qCtx *handler.Context, logger *zap.Logger) (earlyStop bool, err error) {
	err = w.e.Exec(ctx, qCtx)
	return false, err
}

type warpESExec struct {
	e handler.ESExecutable
}

func (w *warpESExec) ExecCmd(ctx context.Context, qCtx *handler.Context, logger *zap.Logger) (earlyStop bool, err error) {
	earlyStop, err = w.e.ExecES(ctx, qCtx)
	return earlyStop, err
}
