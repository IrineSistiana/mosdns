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

package pipeline

import (
	"context"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
)

const PluginType = "pipeline"

func init() {
	handler.RegInitFunc(PluginType, Init, func() interface{} { return new(Args) })
}

var _ handler.ExecutablePlugin = (*pipelineRouter)(nil)

type pipelineRouter struct {
	*handler.BP

	args *Args
}

type Args struct {
	Pipe []string `yaml:"pipe"`
}

func Init(bp *handler.BP, args interface{}) (p handler.Plugin, err error) {
	return newPipelineRouter(bp, args.(*Args))
}

func newPipelineRouter(bp *handler.BP, args *Args) (*pipelineRouter, error) {
	return &pipelineRouter{
		BP:   bp,
		args: args,
	}, nil
}

func (pr *pipelineRouter) Exec(ctx context.Context, qCtx *handler.Context) (err error) {
	return handler.NewPipeContext(pr.args.Pipe, pr.L()).ExecNextPlugin(ctx, qCtx)
}
