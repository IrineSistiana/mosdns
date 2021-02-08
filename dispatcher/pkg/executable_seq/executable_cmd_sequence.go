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
	"go.uber.org/zap"
	"reflect"
	"strings"
)

type executablePluginTag struct {
	s string
}

func (t executablePluginTag) ExecCmd(ctx context.Context, qCtx *handler.Context, logger *zap.Logger) (goTwo string, earlyStop bool, err error) {
	p, err := handler.GetPlugin(t.s)
	if err != nil {
		return "", false, err
	}

	logger.Debug("exec executable plugin", qCtx.InfoField(), zap.String("exec", t.s))
	earlyStop, err = p.ExecES(ctx, qCtx)
	return "", earlyStop, err
}

type IfBlockConfig struct {
	If    []string      `yaml:"if"`
	IfAnd []string      `yaml:"if_and"`
	Exec  []interface{} `yaml:"exec"`
	Goto  string        `yaml:"goto"`
}

type matcher struct {
	tag    string
	negate bool
}

func paresMatcher(s []string) []matcher {
	m := make([]matcher, 0, len(s))
	for _, tag := range s {
		if strings.HasPrefix(tag, "!") {
			m = append(m, matcher{tag: strings.TrimPrefix(tag, "!"), negate: true})
		} else {
			m = append(m, matcher{tag: tag})
		}
	}
	return m
}

type IfBlock struct {
	ifMatcher     []matcher
	ifAndMatcher  []matcher
	executableCmd ExecutableCmd
	goTwo         string
}

func (b *IfBlock) ExecCmd(ctx context.Context, qCtx *handler.Context, logger *zap.Logger) (goTwo string, earlyStop bool, err error) {
	if len(b.ifMatcher) > 0 {
		If, err := ifCondition(ctx, qCtx, logger, b.ifMatcher, false)
		if err != nil {
			return "", false, err
		}
		if If == false {
			return "", false, nil // if case returns false, skip this block.
		}
	}

	if len(b.ifAndMatcher) > 0 {
		If, err := ifCondition(ctx, qCtx, logger, b.ifAndMatcher, true)
		if err != nil {
			return "", false, err
		}
		if If == false {
			return "", false, nil
		}
	}

	// exec
	if b.executableCmd != nil {
		goTwo, earlyStop, err = b.executableCmd.ExecCmd(ctx, qCtx, logger)
		if err != nil {
			return "", false, err
		}
		if len(goTwo) != 0 || earlyStop {
			return goTwo, earlyStop, nil
		}
	}

	// goto
	if len(b.goTwo) != 0 { // if block has a goto, return it
		return b.goTwo, false, nil
	}

	return "", false, nil
}

func ifCondition(ctx context.Context, qCtx *handler.Context, logger *zap.Logger, p []matcher, isAnd bool) (ok bool, err error) {
	if len(p) == 0 {
		return false, err
	}

	for _, m := range p {
		mp, err := handler.GetPlugin(m.tag)
		if err != nil {
			return false, err
		}
		matched, err := mp.Match(ctx, qCtx)
		if err != nil {
			return false, err
		}
		logger.Debug("exec matcher plugin", qCtx.InfoField(), zap.String("exec", m.tag), zap.Bool("result", matched))

		res := matched != m.negate
		if !isAnd && res == true {
			return true, nil // or: if one of the case is true, skip others.
		}
		if isAnd && res == false {
			return false, nil // and: if one of the case is false, skip others.
		}

		ok = res
	}
	return ok, nil
}

func ParseIfBlock(in map[string]interface{}) (*IfBlock, error) {
	c := new(IfBlockConfig)
	err := handler.WeakDecode(in, c)
	if err != nil {
		return nil, err
	}

	b := &IfBlock{
		ifMatcher:    paresMatcher(c.If),
		ifAndMatcher: paresMatcher(c.IfAnd),
		goTwo:        c.Goto,
	}

	if len(c.Exec) != 0 {
		ecs, err := ParseExecutableCmdSequence(c.Exec)
		if err != nil {
			return nil, err
		}
		b.executableCmd = ecs
	}

	return b, nil
}

type ExecutableCmdSequence struct {
	c []ExecutableCmd
}

func ParseExecutableCmdSequence(in []interface{}) (*ExecutableCmdSequence, error) {
	es := &ExecutableCmdSequence{c: make([]ExecutableCmd, 0, len(in))}
	for i, v := range in {
		ec, err := parseExecutableCmd(v)
		if err != nil {
			return nil, fmt.Errorf("invalid cmd #%d: %w", i, err)
		}
		es.c = append(es.c, ec)
	}
	return es, nil
}

func parseExecutableCmd(in interface{}) (ExecutableCmd, error) {
	switch v := in.(type) {
	case string:
		return &executablePluginTag{s: v}, nil
	case map[string]interface{}:
		switch {
		case hasKey(v, "if") || hasKey(v, "if_and"): // if block
			ec, err := ParseIfBlock(v)
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
func (es *ExecutableCmdSequence) ExecCmd(ctx context.Context, qCtx *handler.Context, logger *zap.Logger) (goTwo string, earlyStop bool, err error) {
	for _, cmd := range es.c {
		goTwo, earlyStop, err = cmd.ExecCmd(ctx, qCtx, logger)
		if err != nil {
			return "", false, err
		}
		if len(goTwo) != 0 || earlyStop {
			return goTwo, earlyStop, nil
		}
	}

	return "", false, nil
}

func (es *ExecutableCmdSequence) Len() int {
	return len(es.c)
}
