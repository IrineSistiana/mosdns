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

package handler

import (
	"context"
	"fmt"
	"go.uber.org/zap"
)

// ContextPlugin
type ContextPlugin interface {
	Plugin
	ContextConnector
}

type ContextConnector interface {
	// Connect connects this ContextPlugin to its predecessor.
	Connect(ctx context.Context, qCtx *Context, pipeCtx *PipeContext) (err error)
}

type PipeContext struct {
	logger *zap.Logger
	s      []string

	index int
}

func NewPipeContext(s []string, logger *zap.Logger) *PipeContext {
	return &PipeContext{s: s, logger: logger}
}

func (c *PipeContext) ExecNextPlugin(ctx context.Context, qCtx *Context) error {
	for c.index < len(c.s) {
		tag := c.s[c.index]
		p, err := GetPlugin(tag)
		if err != nil {
			return err
		}
		c.index++
		switch {
		case p.Is(PITContextConnector):
			c.logger.Debug("exec context plugin", qCtx.InfoField(), zap.String("exec", tag))
			return p.Connect(ctx, qCtx, c)
		case p.Is(PITESExecutable):
			c.logger.Debug("exec executable plugin", qCtx.InfoField(), zap.String("exec", tag))
			earlyStop, err := p.ExecES(ctx, qCtx)
			if earlyStop || err != nil {
				return err
			}
		default:
			return fmt.Errorf("plugin %s class err", tag)
		}
	}
	return nil
}
