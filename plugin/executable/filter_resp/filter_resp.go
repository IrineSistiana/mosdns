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

package filter_resp

import (
	"context"
	"fmt"
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
	"github.com/miekg/dns"
	"strconv"
	"strings"
)

const (
	PluginType = "filter_resp"
)

func init() {
	sequence.MustRegExecQuickSetup(PluginType, QuickSetup)
}

var _ sequence.Executable = (*FilterResp)(nil)

type FilterResp struct {
	rrtype uint16
}

func QuickSetup(bq sequence.BQ, s string) (any, error) {
	ts := strings.TrimSpace(s)
	ti, err := strconv.Atoi(ts)
	if err != nil {
		return nil, fmt.Errorf("filter_resp needs one integer argument")
	}

	return NewFilterResp(uint16(ti)), nil
}

func NewFilterResp(rrt uint16) *FilterResp {
	return &FilterResp{rrt}
}

func (f *FilterResp) Exec(ctx context.Context, qCtx *query_context.Context) error {
	r := qCtx.R()
	if r == nil {
		return nil
	}

	qname := qCtx.QQuestion().Name
	var as []dns.RR
	for _, a := range r.Answer {
		h := a.Header()
		if h.Rrtype == f.rrtype {
			continue
		}

		if f.rrtype == dns.TypeCNAME {
			h.Name = qname
		}
		as = append(as, a)
	}
	r.Answer = as

	return nil
}
