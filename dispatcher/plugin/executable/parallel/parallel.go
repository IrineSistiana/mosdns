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

package parallel

import (
	"context"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
)

const PluginType = "parallel"

func init() {
	handler.RegInitFunc(PluginType, Init, func() interface{} { return new(Args) })
}

var _ handler.ExecutablePlugin = (*parallel)(nil)

type parallel struct {
	*handler.BP

	ps *handler.ParallelECS
}

type Args = handler.ParallelECSConfig

func Init(bp *handler.BP, args interface{}) (p handler.Plugin, err error) {
	return newParallel(bp, args.(*Args))
}

func newParallel(bp *handler.BP, args *Args) (*parallel, error) {
	ps, err := handler.ParseParallelECS(args.Parallel)
	if err != nil {
		return nil, err
	}

	return &parallel{
		BP: bp,
		ps: ps,
	}, nil
}

func (p *parallel) Exec(ctx context.Context, qCtx *handler.Context) (err error) {
	return handler.WalkExecutableCmd(ctx, qCtx, p.L(), p.ps)
}
