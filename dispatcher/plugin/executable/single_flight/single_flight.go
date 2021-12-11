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

package single_flight

import (
	"context"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/pkg/single_flight"
)

const (
	PluginType = "single_flight"
)

func init() {
	handler.RegInitFunc(PluginType, Init, func() interface{} { return new(Args) })
	handler.MustRegPlugin(&SingleFlightPlugin{BP: handler.NewBP("_single_flight", PluginType)}, true)
}

type Args struct{}

func Init(bp *handler.BP, args interface{}) (p handler.Plugin, err error) {
	return &SingleFlightPlugin{
		BP:  bp,
		sfg: new(single_flight.SingleFlight),
	}, err
}

type SingleFlightPlugin struct {
	*handler.BP
	sfg *single_flight.SingleFlight
}

var _ handler.ExecutablePlugin = (*SingleFlightPlugin)(nil)

func (sf *SingleFlightPlugin) Exec(ctx context.Context, qCtx *handler.Context, next handler.ExecutableChainNode) error {
	return sf.sfg.Exec(ctx, qCtx, next)
}
