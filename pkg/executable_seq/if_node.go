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

package executable_seq

import (
	"context"
	"fmt"
	"github.com/IrineSistiana/mosdns/v4/pkg/query_context"
	"github.com/Knetic/govaluate"
	"go.uber.org/zap"
	"sync"
)

// ConditionNodeConfig is a config to build a ConditionNode.
type ConditionNodeConfig struct {
	If string `yaml:"if"`

	// See BuildExecutableLogicTree.
	Exec     interface{} `yaml:"exec"`
	ElseExec interface{} `yaml:"else_exec"`
}

// ConditionNode implement handler.ExecutableChainNode.
// Internal ConditionNode.ExecutableNode will also be linked by
// LinkPrevious and LinkNext.
type ConditionNode struct {
	ConditionMatcher   Matcher // if ConditionMatcher is nil, ConditionNode is a no-op.
	ExecutableNode     ExecutableChainNode
	ElseExecutableNode ExecutableChainNode

	next ExecutableChainNode
}

func (b *ConditionNode) Next() ExecutableChainNode {
	return b.next
}

func (b *ConditionNode) LinkNext(n ExecutableChainNode) {
	b.next = n
	if b.ExecutableNode != nil {
		LastNode(b.ExecutableNode).LinkNext(n)
	}
	if b.ElseExecutableNode != nil {
		LastNode(b.ElseExecutableNode).LinkNext(n)
	}
}

func ParseConditionNode(
	cfg *ConditionNodeConfig,
	logger *zap.Logger,
	execs map[string]Executable,
	matchers map[string]Matcher,
) (*ConditionNode, error) {
	cn := new(ConditionNode)

	cm, err := newConditionMatcher(logger, cfg.If, matchers)
	if err != nil {
		return nil, err
	}
	cn.ConditionMatcher = cm

	if cfg.Exec != nil {
		cn.ExecutableNode, err = BuildExecutableLogicTree(cfg.Exec, logger, execs, matchers)
		if err != nil {
			return nil, fmt.Errorf("failed to parse exec command: %w", err)
		}
	}
	if cfg.ElseExec != nil {
		cn.ElseExecutableNode, err = BuildExecutableLogicTree(cfg.ElseExec, logger, execs, matchers)
		if err != nil {
			return nil, fmt.Errorf("failed to parse else_exec command: %w", err)
		}
	}

	return cn, nil
}

type conditionMatcher struct {
	lg           *zap.Logger
	expr         *govaluate.EvaluableExpression
	matchers     map[string]Matcher
	paramsPHPool sync.Pool
}

func newConditionMatcher(lg *zap.Logger, s string, matchers map[string]Matcher) (*conditionMatcher, error) {
	cm := &conditionMatcher{
		lg:           lg,
		matchers:     make(map[string]Matcher),
		paramsPHPool: sync.Pool{},
	}

	expr, err := govaluate.NewEvaluableExpression(s)
	if err != nil {
		return nil, err
	}

	cm.expr = expr
	vs := expr.Vars()
	for _, tag := range vs {
		m := matchers[tag]
		if m == nil {
			return nil, fmt.Errorf("cannot find matcher %s", tag)
		}
		cm.matchers[tag] = m
	}

	// params type check
	expr.ChecksTypes = true
	params := make(govaluate.MapParameters)
	for tag := range cm.matchers {
		params[tag] = true
	}
	if _, err := expr.Eval(params); err != nil {
		return nil, fmt.Errorf("invalid param, %w", err)
	}

	return cm, nil
}

type exprParamsPlaceHolder struct {
	m map[string]func() (bool, error)
}

func (e *exprParamsPlaceHolder) Get(name string) (interface{}, error) {
	f, ok := e.m[name]
	if !ok {
		return nil, fmt.Errorf("cannot find var %s", name)
	}
	return f()
}

func (m *conditionMatcher) Match(ctx context.Context, qCtx *query_context.Context) (bool, error) {
	paramsPH, ok := m.paramsPHPool.Get().(*exprParamsPlaceHolder)
	if !ok {
		paramsPH = &exprParamsPlaceHolder{m: make(map[string]func() (bool, error))}
	}
	defer m.paramsPHPool.Put(paramsPH)

	for tag, matcher := range m.matchers {
		tag := tag
		matcher := matcher
		paramsPH.m[tag] = func() (bool, error) {
			res, err := matcher.Match(ctx, qCtx)
			if err != nil {
				return false, err
			}
			m.lg.Debug("matcher result", qCtx.InfoField(), zap.String("tag", tag), zap.Bool("result", res))
			return res, nil
		}
	}
	out, err := m.expr.Eval(paramsPH)
	if err != nil {
		return false, err
	}
	res := out.(bool)
	m.lg.Debug(
		"condition matcher result",
		qCtx.InfoField(),
		zap.String("expr", m.expr.String()),
		zap.Bool("result", res),
	)
	return res, nil
}

func (b *ConditionNode) Exec(ctx context.Context, qCtx *query_context.Context, next ExecutableChainNode) (err error) {
	if b.ConditionMatcher != nil {
		ok, err := b.ConditionMatcher.Match(ctx, qCtx)
		if err != nil {
			return fmt.Errorf("matcher failed: %w", err)
		}
		if ok && b.ExecutableNode != nil {
			return ExecChainNode(ctx, qCtx, b.ExecutableNode)
		} else if !ok && b.ElseExecutableNode != nil {
			return ExecChainNode(ctx, qCtx, b.ElseExecutableNode)
		}
	}

	return ExecChainNode(ctx, qCtx, next)
}
