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

package bufsize

import (
	"context"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/handler"
)

const PluginType = "bufsize"

func init() {
	handler.RegInitFunc(PluginType, Init, func() interface{} { return new(Args) })
}

type Args struct {
	Size uint16 `yaml:"size"` // The maximum UDP Size. Default value is 512, and the value should be within 512 - 4096.
}

var _ handler.ExecutablePlugin = (*bufSize)(nil)

type bufSize struct {
	*handler.BP
	size uint16
}

func (b *bufSize) getSize() uint16 {
	if b.size < 512 {
		return 512
	}
	if b.size > 4096 {
		return 4096
	}
	return b.size
}

func (b *bufSize) Exec(ctx context.Context, qCtx *handler.Context, next handler.ExecutableChainNode) error {
	q := qCtx.Q()
	if opt := q.IsEdns0(); opt != nil {
		maxSize := b.getSize()
		if opt.UDPSize() > maxSize {
			opt.SetUDPSize(maxSize)
		}
	}

	return handler.ExecChainNode(ctx, qCtx, next)
}

func Init(bp *handler.BP, args interface{}) (p handler.Plugin, err error) {
	return &bufSize{
		BP:   bp,
		size: args.(*Args).Size,
	}, nil
}
