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

package rcode

import (
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
	"github.com/IrineSistiana/mosdns/v5/plugin/matcher/base_int"
)

const PluginType = "rcode"

func init() {
	sequence.MustRegMatchQuickSetup(PluginType, base_int.QuickSetup(matchRcode))
}

func matchRcode(qCtx *query_context.Context, m base_int.IntMatcher) (bool, error) {
	r := qCtx.R()
	if r == nil {
		return false, nil
	}
	return m.Has(r.Rcode), nil
}
