//     Copyright (C) 2020, IrineSistiana
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

package handler

import (
	"context"
	"fmt"
	"github.com/IrineSistiana/mosdns/dispatcher/mlog"
)

type RouterPlugin interface {
	Plugin
	Do(ctx context.Context, qCtx *Context) (next string, err error)
}

const (
	// IterationLimit is to prevent endless loops.
	IterationLimit = 50
)

// Walk walks into RouterPlugin. entry must be an RouterPlugin tag.
// Walk will stop and return
// when last RouterPlugin.Do() returns:
// 1. An empty tag.
// 2. An error.
func Walk(ctx context.Context, qCtx *Context, entry string) (err error) {
	mlog.Entry().Debugf("%v: start entry router plugin %s", qCtx, entry)
	defer mlog.Entry().Debugf("%v: entry %s returned", qCtx, entry)

	nextTag := entry
	for i := 0; i < IterationLimit; i++ {
		// check ctx
		if err := ctx.Err(); err != nil {
			return err
		}

		p, err := GetRouterPlugin(nextTag) // get next plugin
		if err != nil {
			return err
		}

		nextTag, err = p.Do(ctx, qCtx)
		if err != nil {
			return NewErrFromTemplate(ETPluginErr, p.Tag(), err)
		}
		if len(nextTag) == 0 { // end of the plugin chan
			return nil
		}
		mlog.Entry().Debugf("%v: next router plugin %s", qCtx, nextTag)
	}

	return fmt.Errorf("length of plugin execution sequence reached limit %d", IterationLimit)
}
