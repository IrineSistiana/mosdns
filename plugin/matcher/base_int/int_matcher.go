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

package base_int

import (
	"context"
	"fmt"
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
	"strconv"
	"strings"
)

var _ sequence.Matcher = (*Matcher)(nil)

type MatchFunc func(qCtx *query_context.Context, m IntMatcher) (bool, error)

type Matcher struct {
	match MatchFunc
	m     IntMatcher
}

func (m *Matcher) Match(_ context.Context, qCtx *query_context.Context) (bool, error) {
	return m.match(qCtx, m.m)
}

func NewMatcher(args []int, f MatchFunc) (*Matcher, error) {
	m := &Matcher{
		match: f,
		m:     make(map[int]struct{}),
	}
	for _, i := range args {
		m.m[i] = struct{}{}
	}
	return m, nil
}

// ParseQuickSetupArgs parses numbers to Args.
// Format: "[int]..."
func ParseQuickSetupArgs(s string) ([]int, error) {
	args := make([]int, 0)
	for i, s := range strings.Fields(s) {
		n, err := strconv.Atoi(s)
		if err != nil {
			return nil, fmt.Errorf("arg #%d is not an int, %w", i, err)
		}
		args = append(args, n)
	}
	return args, nil
}

// QuickSetup returns a sequence.QuickSetupFunc.
func QuickSetup(f MatchFunc) func(_ sequence.BQ, s string) (any, error) {
	return func(_ sequence.BQ, s string) (any, error) {
		args, err := ParseQuickSetupArgs(s)
		if err != nil {
			return nil, fmt.Errorf("invalid args, %w", err)
		}
		return NewMatcher(args, f)
	}
}

type IntMatcher map[int]struct{}

func (m IntMatcher) Has(i int) bool {
	_, ok := m[i]
	return ok
}
