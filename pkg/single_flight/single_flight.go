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
	"fmt"
	"github.com/IrineSistiana/mosdns/v4/pkg/executable_seq"
	"github.com/IrineSistiana/mosdns/v4/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v4/pkg/utils"
	"golang.org/x/sync/singleflight"
	"reflect"
)

type SingleFlight struct {
	g singleflight.Group
}

func (sf *SingleFlight) Exec(ctx context.Context, qCtx *query_context.Context, next executable_seq.ExecutableChainNode) error {
	if v := reflect.ValueOf(next); v.Kind() == reflect.Ptr {
		key, err := utils.GetMsgKeyWithInt64Salt(qCtx.Q(), int64(v.Pointer()))
		if err != nil {
			return fmt.Errorf("failed to get msg key, %w", err)
		}

		qCtxCopy := qCtx.Copy()
		v, err, _ := sf.g.Do(key, func() (interface{}, error) {
			defer sf.g.Forget(key)
			err := executable_seq.ExecChainNode(ctx, qCtxCopy, next)
			return qCtxCopy, err
		})

		if err != nil {
			return err
		}

		qCtxUnsafe := v.(*query_context.Context)

		// Returned qCtxUnsafe may also be returned to other goroutines.
		// Make a deep copy of it to qCtx. Then we can modify it safely.
		qid := qCtx.Q().Id
		qCtxUnsafe.CopyTo(qCtx)
		if r := qCtx.R(); r != nil { // Make sure msg IDs are consistent.
			r.Id = qid
		}

		return nil
	}

	return executable_seq.ExecChainNode(ctx, qCtx, next)
}
