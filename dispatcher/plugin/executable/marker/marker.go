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

package marker

import (
	"context"
	"errors"
	"github.com/IrineSistiana/mosdns/v3/dispatcher/handler"
	"sync/atomic"
)

const PluginType = "marker"

func init() {
	handler.RegInitFunc(PluginType, Init, func() interface{} { return new(Args) })
}

var pluginId uint32

var _ handler.ExecutablePlugin = (*markerPlugin)(nil)
var _ handler.MatcherPlugin = (*markerPlugin)(nil)

type markerPlugin struct {
	*handler.BP
	markId uint
}

func (s *markerPlugin) Match(_ context.Context, qCtx *handler.Context) (matched bool, err error) {
	return qCtx.HasMark(s.markId), nil
}

type Args struct{}

// Exec implements handler.Executable.
func (s *markerPlugin) Exec(ctx context.Context, qCtx *handler.Context, next handler.ExecutableChainNode) error {
	qCtx.AddMark(uint(s.markId))
	return handler.ExecChainNode(ctx, qCtx, next)
}

func Init(bp *handler.BP, _ interface{}) (p handler.Plugin, err error) {
	markId := atomic.AddUint32(&pluginId, 1)
	if markId == 0 {
		return nil, errors.New("mark id overflowed")
	}

	return &markerPlugin{
		BP:     bp,
		markId: uint(markId),
	}, nil
}
