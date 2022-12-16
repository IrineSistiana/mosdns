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

package sequence

import (
	"context"
	"errors"
	"fmt"
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"io"
)

type chainNode struct {
	matches []Matcher // may be empty, indicates this node has no match specified.

	// at least one of e or re is not nil.
	e  Executable
	re RecursiveExecutable
}

type ChainWalker struct {
	p        int
	chain    []*chainNode
	jumpBack *ChainWalker
}

func newChainWalker(chain []*chainNode, jumpBack *ChainWalker) ChainWalker {
	return ChainWalker{
		chain:    chain,
		jumpBack: jumpBack,
	}
}

func (w *ChainWalker) ExecNext(ctx context.Context, qCtx *query_context.Context) error {
	p := w.p
	// Evaluate rules' matchers in loop.
checkMatchesLoop:
	for p < len(w.chain) {
		n := w.chain[p]

		for _, match := range n.matches {
			ok, err := match.Match(ctx, qCtx)
			if err != nil {
				return err
			}
			if !ok {
				// Skip this node if condition was not matched.
				p++
				continue checkMatchesLoop
			}
		}

		// Exec rules' executables in loop, or in stack if it is a recursive executable.
		switch {
		case n.e != nil:
			if err := n.e.Exec(ctx, qCtx); err != nil {
				return err
			}
			p++
			continue
		case n.re != nil:
			next := ChainWalker{
				p:        p + 1,
				chain:    w.chain,
				jumpBack: w.jumpBack,
			}
			return n.re.Exec(ctx, qCtx, next)
		default:
			panic("n cannot be executed")
		}
	}

	if w.jumpBack != nil { // End of chain, time to jump back.
		return w.jumpBack.ExecNext(ctx, qCtx)
	}

	// EoC.
	return nil
}

func (w *ChainWalker) nop() bool {
	return w.p >= len(w.chain)
}

func (s *sequence) buildChain(bq BQ, rs []RuleConfig) error {
	c := make([]*chainNode, 0, len(rs))
	for ri, r := range rs {
		n, err := s.newNode(bq, r, ri)
		if err != nil {
			return fmt.Errorf("failed to init rule #%d, %w", ri, err)
		}
		c = append(c, n)
	}
	s.chain = c
	return nil
}

func (s *sequence) newNode(bq BQ, r RuleConfig, ri int) (*chainNode, error) {
	n := new(chainNode)

	// init matches
	for mi, mc := range r.Matches {
		m, err := s.newMatcher(bq, mc, ri, mi)
		if err != nil {
			return nil, fmt.Errorf("failed to init matcher #%d, %w", mi, err)
		}
		n.matches = append(n.matches, m)
	}

	// init exec
	e, re, err := s.newExec(bq, r, ri)
	if err != nil {
		return nil, fmt.Errorf("failed to init exec, %w", err)
	}
	n.e = e
	n.re = re
	return n, nil
}

func (s *sequence) newMatcher(bq BQ, mc MatchConfig, ri, mi int) (Matcher, error) {
	var m Matcher
	switch {
	case len(mc.Tag) > 0:
		m, _ = bq.M().GetPlugin(mc.Tag).(Matcher)
		if m == nil {
			return nil, fmt.Errorf("can not find matcher %s", mc.Tag)
		}
		if qc, ok := m.(QuickConfigurableMatch); ok {
			v, err := qc.QuickConfigureMatch(mc.Args)
			if err != nil {
				return nil, fmt.Errorf("fail to configure plugin %s, %w", mc.Tag, err)
			}
			m = v
		}

	case len(mc.Type) > 0:
		f := GetMatchQuickSetup(mc.Type)
		if f == nil {
			return nil, fmt.Errorf("invalid matcher type %s", mc.Type)
		}
		p, err := f(NewBQ(bq.M(), bq.L().Named(fmt.Sprintf("r%d.m%d", ri, mi))), mc.Args)
		if err != nil {
			return nil, fmt.Errorf("failed to init matcher, %w", err)
		}
		s.anonymousPlugins = append(s.anonymousPlugins, p)
		m = p
	}
	if m == nil {
		return nil, errors.New("missing args")
	}
	if mc.Reverse {
		m = reverseMatcher(m)
	}
	return m, nil
}

func (s *sequence) newExec(bq BQ, rc RuleConfig, ri int) (Executable, RecursiveExecutable, error) {
	var exec any
	switch {
	case len(rc.Tag) > 0:
		p := bq.M().GetPlugin(rc.Tag)
		if p == nil {
			return nil, nil, fmt.Errorf("can not find executable %s", rc.Tag)
		}
		if qc, ok := p.(QuickConfigurableExec); ok {
			v, err := qc.QuickConfigureExec(rc.Args)
			if err != nil {
				return nil, nil, fmt.Errorf("fail to configure plugin %s, %w", rc.Tag, err)
			}
			exec = v
		} else {
			exec = p
		}

	case len(rc.Type) > 0:
		f := GetExecQuickSetup(rc.Type)
		if f == nil {
			return nil, nil, fmt.Errorf("invalid executable type %s", rc.Type)
		}
		v, err := f(NewBQ(bq.M(), bq.L().Named(fmt.Sprintf("r%d", ri))), rc.Args)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to init executable, %w", err)
		}
		s.anonymousPlugins = append(s.anonymousPlugins, v)
		exec = v
	default:
		return nil, nil, errors.New("missing args")
	}

	e, _ := exec.(Executable)
	re, _ := exec.(RecursiveExecutable)

	if re == nil && e == nil {
		return nil, nil, errors.New("invalid args, initialized object is not executable")
	}
	return e, re, nil
}

func closePlugin(p any) {
	if c, ok := p.(io.Closer); ok {
		_ = c.Close()
	}
}

func reverseMatcher(m Matcher) Matcher {
	return reverseMatch{m: m}
}

type reverseMatch struct {
	m Matcher
}

func (r reverseMatch) Match(ctx context.Context, qCtx *query_context.Context) (bool, error) {
	ok, err := r.m.Match(ctx, qCtx)
	if err != nil {
		return false, err
	}
	return !ok, nil
}
