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
)

type Checker interface {
	Match(ctx context.Context, qCtx *Context) (matched bool, err error)
}

type redirectPlugin struct {
	config         *Config
	checker        Checker
	next, redirect string
}

// NewRedirectPlugin returns a redirectPlugin.
// redirectPlugin is a plugin that can redirect qCtx to other plugin under certain condition.
// It should not modify qCtx.
// It has two pre-set args: `next` and `redirect`.
// If checker.Match() returns true, the plugin tag from `next` will be returned in redirectPlugin.Next().
// Otherwise, `redirect`.
func NewRedirectPlugin(config *Config, checker Checker, next, redirect string) Plugin {
	return &redirectPlugin{
		config:   config,
		checker:  checker,
		next:     next,
		redirect: redirect,
	}
}

func (c *redirectPlugin) Tag() string {
	return c.config.Tag
}

func (c *redirectPlugin) Type() string {
	return c.config.Type
}

func (c *redirectPlugin) Do(ctx context.Context, qCtx *Context) (err error) {
	return nil
}

func (c *redirectPlugin) Next(ctx context.Context, qCtx *Context) (next string, err error) {
	matched, err := c.checker.Match(ctx, qCtx)
	if err != nil {
		return "", err
	}

	if matched {
		next = c.redirect
	} else {
		next = c.next
	}
	return next, nil
}
