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

package single_flight

import (
	"context"
	"github.com/IrineSistiana/mosdns/v4/coremain"
	"github.com/IrineSistiana/mosdns/v4/pkg/executable_seq"
	"github.com/IrineSistiana/mosdns/v4/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v4/pkg/single_flight"
)

const (
	PluginType = "single_flight"
)

func init() {
	coremain.RegNewPluginFunc(PluginType, Init, func() interface{} { return new(Args) })
	coremain.RegNewPersetPluginFunc("_single_flight", func(bp *coremain.BP) (coremain.Plugin, error) {
		return NewSF(bp), nil
	})
}

type Args struct{}

func Init(bp *coremain.BP, args interface{}) (p coremain.Plugin, err error) {
	return NewSF(bp), nil
}

type SingleFlightPlugin struct {
	*coremain.BP
	sf *single_flight.SingleFlight
}

var _ coremain.ExecutablePlugin = (*SingleFlightPlugin)(nil)

func NewSF(bp *coremain.BP) *SingleFlightPlugin {
	return &SingleFlightPlugin{
		BP: bp,
		sf: new(single_flight.SingleFlight),
	}
}

func (p *SingleFlightPlugin) Exec(ctx context.Context, qCtx *query_context.Context, next executable_seq.ExecutableChainNode) error {
	return p.sf.Exec(ctx, qCtx, next)
}
