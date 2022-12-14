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

package mark

import (
	"context"
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
	"strconv"
	"strings"
)

const PluginType = "mark"

func init() {
	sequence.MustRegExecQuickSetup(PluginType, func(_ sequence.BQ, args string) (any, error) {
		return newMarker(args)
	})
	sequence.MustRegMatchQuickSetup(PluginType, func(_ sequence.BQ, args string) (sequence.Matcher, error) {
		return newMarker(args)
	})
}

var _ sequence.Executable = (*mark)(nil)
var _ sequence.Matcher = (*mark)(nil)

type mark struct {
	m []uint32
}

func (m *mark) Match(_ context.Context, qCtx *query_context.Context) (bool, error) {
	for _, u := range m.m {
		if qCtx.HasMark(u) {
			return true, nil
		}
	}
	return false, nil
}

func (m *mark) Exec(_ context.Context, qCtx *query_context.Context) error {
	for _, u := range m.m {
		qCtx.SetMark(u)
	}
	return nil
}

// newMarker format: [uint32_mark]...
// "uint32_mark" is an uint32 defined as Go syntax for integer literals.
// e.g. "111", "0b111", "0o111", "0xfff".
func newMarker(s string) (*mark, error) {
	var m []uint32
	for _, ms := range strings.Fields(s) {
		n, err := strconv.ParseUint(ms, 10, 32)
		if err != nil {
			return nil, err
		}
		m = append(m, uint32(n))
	}
	return &mark{m: m}, nil
}
