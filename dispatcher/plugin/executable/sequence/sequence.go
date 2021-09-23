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
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/dispatcher/pkg/executable_seq"
)

const PluginType = "sequence"

func init() {
	handler.RegInitFunc(PluginType, Init, func() interface{} { return new(Args) })

	handler.MustRegPlugin(&end{BP: handler.NewBP("_end", PluginType)}, true)
}

var _ handler.ESExecutablePlugin = (*sequenceRouter)(nil)

type sequenceRouter struct {
	*handler.BP

	ecs executable_seq.ExecutableNode
}

type Args struct {
	Exec interface{} `yaml:"exec"`
}

func Init(bp *handler.BP, args interface{}) (p handler.Plugin, err error) {
	return newSequencePlugin(bp, args.(*Args))
}

func newSequencePlugin(bp *handler.BP, args *Args) (*sequenceRouter, error) {
	ecs, err := executable_seq.ParseExecutableNode(args.Exec)
	if err != nil {
		return nil, fmt.Errorf("invalid exec squence: %w", err)
	}

	return &sequenceRouter{
		BP:  bp,
		ecs: ecs,
	}, nil
}

func (s *sequenceRouter) Exec(ctx context.Context, qCtx *handler.Context) (err error) {
	_, err = s.ecs.Exec(ctx, qCtx, s.L())
	return err
}

func (s *sequenceRouter) ExecES(ctx context.Context, qCtx *handler.Context) (earlyStop bool, err error) {
	return executable_seq.ExecRoot(ctx, qCtx, s.L(), s.ecs)
}

var _ handler.ESExecutablePlugin = (*end)(nil)

// end is a no-op handler.Executable and handler.ESExecutable.
// It can be used as a stop sign in the handler.ExecutableNode.
type end struct {
	*handler.BP
}

func (n *end) ExecES(_ context.Context, _ *handler.Context) (earlyStop bool, err error) {
	return true, nil
}
