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

package ttl

import (
	"context"
	"fmt"
	"github.com/IrineSistiana/mosdns/v5/pkg/dnsutils"
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
	"strconv"
	"strings"
)

const (
	PluginType = "ttl"
)

func init() {
	sequence.MustRegExecQuickSetup(PluginType, QuickSetup)
}

var _ sequence.Executable = (*TTL)(nil)

type TTL struct {
	fix uint32
	min uint32
	max uint32
}

func NewTTL(fix, min, max uint32) *TTL {
	return &TTL{
		fix: fix,
		min: min,
		max: max,
	}
}

// QuickSetup format: {[min-max]|[fix]}
// e.g. range "300-600", fixed ttl "5".
func QuickSetup(_ sequence.BQ, s string) (any, error) {
	var f, l, u uint32
	ls, us, ok := strings.Cut(s, "-")
	if ok { // range
		n, err := strconv.ParseUint(ls, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid lower bound, %w", err)
		}
		l = uint32(n)
		n, err = strconv.ParseUint(us, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid upper bound, %w", err)
		}
		u = uint32(n)
	} else { // fixed
		n, err := strconv.ParseUint(s, 10, 32)
		if err != nil {
			return nil, fmt.Errorf("invalid ttl, %w", err)
		}
		f = uint32(n)
	}

	return NewTTL(f, l, u), nil
}

func (t *TTL) Exec(_ context.Context, qCtx *query_context.Context) error {
	if r := qCtx.R(); r != nil {
		if t.fix > 0 {
			dnsutils.SetTTL(r, t.fix)
		} else {
			if t.min > 0 {
				dnsutils.ApplyMinimalTTL(r, t.min)
			}
			if t.max > 0 {
				dnsutils.ApplyMaximumTTL(r, t.max)
			}
		}
	}
	return nil
}
