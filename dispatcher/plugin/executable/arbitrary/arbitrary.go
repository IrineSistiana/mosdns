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

package arbitrary

import (
	"context"
	"fmt"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/pkg/zone_file"
	"strings"
)

const PluginType = "arbitrary"

func init() {
	handler.RegInitFunc(PluginType, Init, func() interface{} { return new(Args) })
}

type Args struct {
	RR []string `yaml:"rr"`
}

var _ handler.ExecutablePlugin = (*arbitraryPlugin)(nil)

type arbitraryPlugin struct {
	*handler.BP
	m *zone_file.Matcher
}

func (p *arbitraryPlugin) Exec(ctx context.Context, qCtx *handler.Context, next handler.ExecutableChainNode) error {
	if r := p.m.Reply(qCtx.Q()); r != nil {
		qCtx.SetResponse(r, handler.ContextStatusResponded)
		return nil
	}
	return handler.ExecChainNode(ctx, qCtx, next)
}

func Init(bp *handler.BP, v interface{}) (p handler.Plugin, err error) {
	args := v.(*Args)
	m := new(zone_file.Matcher)
	for i, s := range args.RR {
		if strings.HasPrefix(s, "ext:") {
			s = strings.TrimPrefix(s, "ext:")
			if err := m.LoadFile(s); err != nil {
				return nil, fmt.Errorf("failed to load zone file #%d %s, %w", i, s, err)
			}
		}
		if err := m.Load(strings.NewReader(s)); err != nil {
			return nil, fmt.Errorf("failed to load rr #%d [%s], %w", i, s, err)
		}
	}
	return &arbitraryPlugin{
		BP: bp,
		m:  m,
	}, nil
}
