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

package bufsize

import (
	"context"
	"github.com/sieveLau/mosdns/v4-maintenance/coremain"
	"github.com/sieveLau/mosdns/v4-maintenance/pkg/executable_seq"
	"github.com/sieveLau/mosdns/v4-maintenance/pkg/query_context"
)

const PluginType = "bufsize"

func init() {
	coremain.RegNewPluginFunc(PluginType, Init, func() interface{} { return new(Args) })
}

type Args struct {
	Size uint16 `yaml:"size"` // The maximum UDP Size. Default value is 512, and the value should be within 512 - 4096.
}

var _ coremain.ExecutablePlugin = (*bufSize)(nil)

type bufSize struct {
	*coremain.BP
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

func (b *bufSize) Exec(ctx context.Context, qCtx *query_context.Context, next executable_seq.ExecutableChainNode) error {
	q := qCtx.Q()
	if opt := q.IsEdns0(); opt != nil {
		maxSize := b.getSize()
		if opt.UDPSize() > maxSize {
			opt.SetUDPSize(maxSize)
		}
	}

	return executable_seq.ExecChainNode(ctx, qCtx, next)
}

func Init(bp *coremain.BP, args interface{}) (p coremain.Plugin, err error) {
	return &bufSize{
		BP:   bp,
		size: args.(*Args).Size,
	}, nil
}
