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
	"github.com/sirupsen/logrus"
)

type RouterPlugin interface {
	Plugin
	Do(ctx context.Context, qCtx *Context) (next string, err error)
}

const (
	// IterationLimit is to prevent endless loops.
	IterationLimit = 50

	// StopSignTag: See Walk().
	StopSignTag = "end"
)

// Walk walks into this RouterPlugin. Walk will stop and return when
// last RouterPlugin.Do() returns:
// 1. An empty tag or StopSignTag.
// 2. An error.
func Walk(ctx context.Context, qCtx *Context, entryTag string) (err error) {
	nextTag := entryTag

	for i := 0; i < IterationLimit; i++ {
		// check ctx
		if err := ctx.Err(); err != nil {
			return err
		}

		p, ok := GetRouterPlugin(nextTag) // get next plugin
		if !ok {
			return NewErrFromTemplate(ETTagNotDefined, nextTag)
		}
		qCtx.Logf(logrus.DebugLevel, "exec plugin %s", p.Tag())

		nextTag, err = p.Do(ctx, qCtx)
		if err != nil {
			return fmt.Errorf("plugin %s reports an err: %w", p.Tag(), err)
		}
		if len(nextTag) == 0 || nextTag == StopSignTag { // end of the plugin chan
			return nil
		}
	}

	return fmt.Errorf("length of plugin execution sequence reached limit %d", IterationLimit)
}
