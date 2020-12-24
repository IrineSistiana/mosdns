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

// ContextPlugin
type ContextPlugin interface {
	Plugin

	// Connect connects this ContextPlugin to its predecessor.
	Connect(ctx context.Context, qCtx *Context, pipeCtx *PipeContext) (err error)
}
type PipeContext struct {
	logger *logrus.Entry
	s      []string

	index int
}

func NewPipeContext(s []string, logger *logrus.Entry) *PipeContext {
	return &PipeContext{s: s, logger: logger}
}

func (c *PipeContext) ExecNextPlugin(ctx context.Context, qCtx *Context) error {
	for c.index < len(c.s) {
		tag := c.s[c.index]
		i, err := GetPlugin(tag)
		if err != nil {
			return err
		}
		c.index++
		switch p := i.(type) {
		case ContextPlugin:
			c.logger.Debugf("%v: exec context plugin %s", qCtx, tag)
			return p.Connect(ctx, qCtx, c)
		case ExecutablePlugin:
			c.logger.Debugf("%v: exec executable plugin %s", qCtx, tag)
			err := p.Exec(ctx, qCtx)
			if err != nil {
				return err
			}
		default:
			return fmt.Errorf("plugin %s has a unsupported class", tag)
		}
	}
	return nil
}
