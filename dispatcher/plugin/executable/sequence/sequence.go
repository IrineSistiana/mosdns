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

package sequence

import (
	"context"
	"fmt"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/pkg/executable_seq"
	"sync"
)

const PluginType = "sequence"

func init() {
	handler.RegInitFunc(PluginType, Init, func() interface{} { return new(Args) })

	handler.MustRegPlugin(&_return{BP: handler.NewBP("_return", PluginType)})

	// TODO: Remove deprecated.
	handler.MustRegPlugin(&_end{BP: handler.NewBP("_end", PluginType)})
}

var _ handler.ExecutablePlugin = (*sequence)(nil)

type sequence struct {
	*handler.BP

	ecs handler.ExecutableChainNode
}

type Args struct {
	Exec interface{} `yaml:"exec"`
}

func Init(bp *handler.BP, args interface{}) (p handler.Plugin, err error) {
	return newSequencePlugin(bp, args.(*Args))
}

func newSequencePlugin(bp *handler.BP, args *Args) (*sequence, error) {
	ecs, err := executable_seq.ParseExecutableNode(args.Exec, bp.L())
	if err != nil {
		return nil, fmt.Errorf("invalid exec squence: %w", err)
	}

	return &sequence{
		BP:  bp,
		ecs: ecs,
	}, nil
}

func (s *sequence) Exec(ctx context.Context, qCtx *handler.Context, next handler.ExecutableChainNode) error {
	if err := handler.ExecChainNode(ctx, qCtx, s.ecs); err != nil {
		return err
	}

	return handler.ExecChainNode(ctx, qCtx, next)
}

var _ handler.ExecutablePlugin = (*_end)(nil)
var _ handler.ExecutablePlugin = (*_return)(nil)

// TODO: Remove deprecated.
type _end struct {
	*handler.BP
	warnOnce sync.Once
}

func (n *_end) Exec(_ context.Context, _ *handler.Context, _ handler.ExecutableChainNode) error {
	n.warnOnce.Do(func() {
		n.L().Warn("_end is deprecated, use _return instead")
	})
	return nil
}

type _return struct {
	*handler.BP
}

func (n *_return) Exec(_ context.Context, _ *handler.Context, _ handler.ExecutableChainNode) error {
	return nil
}
