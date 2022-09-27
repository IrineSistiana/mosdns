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

package arbitrary

import (
	"context"
	"fmt"
	"github.com/IrineSistiana/mosdns/v4/coremain"
	"github.com/IrineSistiana/mosdns/v4/pkg/executable_seq"
	"github.com/IrineSistiana/mosdns/v4/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v4/pkg/zone_file"
	"strings"
)

const PluginType = "arbitrary"

func init() {
	coremain.RegNewPluginFunc(PluginType, Init, func() interface{} { return new(Args) })
}

type Args struct {
	RR []string `yaml:"rr"`
}

var _ coremain.ExecutablePlugin = (*arbitraryPlugin)(nil)

type arbitraryPlugin struct {
	*coremain.BP
	m *zone_file.Matcher
}

func (p *arbitraryPlugin) Exec(ctx context.Context, qCtx *query_context.Context, next executable_seq.ExecutableChainNode) error {
	if r := p.m.Reply(qCtx.Q()); r != nil {
		qCtx.SetResponse(r)
		return nil
	}
	return executable_seq.ExecChainNode(ctx, qCtx, next)
}

func Init(bp *coremain.BP, v interface{}) (p coremain.Plugin, err error) {
	args := v.(*Args)
	m := new(zone_file.Matcher)

	//TODO: Support data provider
	for i, s := range args.RR {
		if err := m.Load(strings.NewReader(s)); err != nil {
			return nil, fmt.Errorf("failed to load rr #%d [%s], %w", i, s, err)
		}
	}
	return &arbitraryPlugin{
		BP: bp,
		m:  m,
	}, nil
}
