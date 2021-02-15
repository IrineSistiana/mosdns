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

func (e earlyStop) ExecCmd(ctx context.Context, qCtx *handler.Context, logger *zap.Logger) (goTwo handler.ESExecutable, earlyStop bool, err error) {
	return nil, true, nil
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

func (ref RefESExecutablePlugin) ExecCmd(ctx context.Context, qCtx *handler.Context, logger *zap.Logger) (goTwo handler.ESExecutable, earlyStop bool, err error) {
	earlyStop, err = ref.ExecES(ctx, qCtx)
	return nil, earlyStop, err
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

// IfBlockConfig can build a IfBlock from human input.
type IfBlockConfig struct {
	If    []string      `yaml:"if"`
	IfAnd []string      `yaml:"if_and"`
	Exec  []interface{} `yaml:"exec"`
	Goto  string        `yaml:"goto"`
}

func paresRefMatcher(s []string) []handler.Matcher {
	m := make([]handler.Matcher, 0, len(s))
	for _, tag := range s {
		if strings.HasPrefix(tag, "!") {
			m = append(m, NagateMatcher(RefMatcherPlugin(strings.TrimPrefix(tag, "!"))))
		} else {
			m = append(m, RefMatcherPlugin(tag))
		}
	}
	return m
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

type IfBlock struct {
	IfMatcher     []handler.Matcher
	IfAndMatcher  []handler.Matcher
	ExecutableCmd ExecutableCmd
	GoTwo         handler.ESExecutable
}

func (b *IfBlock) ExecCmd(ctx context.Context, qCtx *handler.Context, logger *zap.Logger) (goTwo handler.ESExecutable, earlyStop bool, err error) {
	if len(b.IfMatcher) > 0 {
		If, err := utils.BoolLogic(ctx, qCtx, b.IfMatcher, false)
		if err != nil {
			return nil, false, err
		}
		if If == false {
			return nil, false, nil // if case returns false, skip this block.
		}
	}

	if len(b.IfAndMatcher) > 0 {
		If, err := utils.BoolLogic(ctx, qCtx, b.IfAndMatcher, true)
		if err != nil {
			return nil, false, err
		}
		if If == false {
			return nil, false, nil
		}
	}

	// exec
	if b.ExecutableCmd != nil {
		goTwo, earlyStop, err = b.ExecutableCmd.ExecCmd(ctx, qCtx, logger)
		if err != nil {
			return nil, false, err
		}
		if goTwo != nil || earlyStop {
			return goTwo, earlyStop, nil
		}
	}

	// goto
	if b.GoTwo != nil { // if block has a goto, return it
		return b.GoTwo, false, nil
	}

	return nil, false, nil
}

func ParseIfBlock(c *IfBlockConfig) (*IfBlock, error) {
	b := &IfBlock{
		IfMatcher:    paresRefMatcher(c.If),
		IfAndMatcher: paresRefMatcher(c.IfAnd),
	}
	if len(c.Goto) != 0 {
		b.GoTwo = RefESExecutablePlugin(c.Goto)
	}

	if len(c.Exec) != 0 {
		ecs, err := ParseExecutableCmdSequence(c.Exec)
		if err != nil {
			return nil, err
		}
		b.ExecutableCmd = ecs
	}

	return b, nil
}

type ExecutableCmdSequence struct {
	c []ExecutableCmd
}

func ParseExecutableCmdSequence(in []interface{}) (*ExecutableCmdSequence, error) {
	es := &ExecutableCmdSequence{c: make([]ExecutableCmd, 0, len(in))}
	for i, v := range in {
		ec, err := ParseExecutableCmd(v)
		if err != nil {
			return nil, fmt.Errorf("invalid cmd #%d: %w", i, err)
		}
		es.c = append(es.c, ec)
	}
	return es, nil
}

func ParseExecutableCmd(in interface{}) (ExecutableCmd, error) {
	switch v := in.(type) {
	case ExecutableCmd:
		return v, nil
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

// ExecCmd executes the sequence.
func (es *ExecutableCmdSequence) ExecCmd(ctx context.Context, qCtx *handler.Context, logger *zap.Logger) (goTwo handler.ESExecutable, earlyStop bool, err error) {
	for _, cmd := range es.c {
		goTwo, earlyStop, err = cmd.ExecCmd(ctx, qCtx, logger)
		if err != nil {
			return nil, false, err
		}
		if goTwo != nil || earlyStop {
			return goTwo, earlyStop, nil
		}
	}

	return nil, false, nil
}

func (es *ExecutableCmdSequence) Len() int {
	return len(es.c)
}

type warpExec struct {
	e handler.Executable
}

func (w *warpExec) ExecCmd(ctx context.Context, qCtx *handler.Context, logger *zap.Logger) (goTwo handler.ESExecutable, earlyStop bool, err error) {
	err = w.e.Exec(ctx, qCtx)
	return nil, false, err
}

type warpESExec struct {
	e handler.ESExecutable
}

func (w *warpESExec) ExecCmd(ctx context.Context, qCtx *handler.Context, logger *zap.Logger) (goTwo handler.ESExecutable, earlyStop bool, err error) {
	earlyStop, err = w.e.ExecES(ctx, qCtx)
	return nil, earlyStop, err
}
