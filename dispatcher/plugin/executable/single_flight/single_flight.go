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
	"fmt"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/v2/dispatcher/pkg/utils"
	"golang.org/x/sync/singleflight"
)

const (
	PluginType = "single_flight"
)

func init() {
	handler.MustRegPlugin(&singleFlight{BP: handler.NewBP("_single_flight", PluginType)}, true)
}

type singleFlight struct {
	*handler.BP

	singleflight.Group
}

var _ handler.ExecutablePlugin = (*singleFlight)(nil)

func (sf *singleFlight) Exec(ctx context.Context, qCtx *handler.Context, next handler.ExecutableChainNode) error {
	key, err := utils.GetMsgKey(qCtx.Q(), 0)
	if err != nil {
		return fmt.Errorf("failed to get msg key, %w", err)
	}

	qCtxCopy := qCtx.Copy()
	v, err, _ := sf.Group.Do(key, func() (interface{}, error) {
		defer sf.Group.Forget(key)
		err := handler.ExecChainNode(ctx, qCtxCopy, next)
		return qCtxCopy, err
	})

	if err != nil {
		return err
	}

	qCtxUnsafe := v.(*handler.Context)

	// Returned qCtxUnsafe may from another goroutine.
	// Replace qCtx.
	qCtxUnsafe.CopyTo(qCtx)
	if r := qCtx.R(); r != nil { // Make sure msg IDs are consistent.
		r.Id = qCtx.Q().Id
	}

	return nil
}
