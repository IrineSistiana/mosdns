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
	"github.com/IrineSistiana/mosdns/v2/dispatcher/mlog"
	"go.uber.org/zap"
)

var (
	_ Executable = (*PluginWrapper)(nil)
	_ Matcher    = (*PluginWrapper)(nil)
	_ Service    = (*PluginWrapper)(nil)
)

// PluginWrapper wraps the original plugin to avoid extremely frequently
// interface conversion. To access the original plugin, use PluginWrapper.GetPlugin()
// Note: PluginWrapper not implements Executable.
// It automatically converts Executable to ESExecutable.
type PluginWrapper struct {
	p Plugin
	e Executable
	m Matcher
	s Service
}

func NewPluginWrapper(gp Plugin) *PluginWrapper {
	w := new(PluginWrapper)
	w.p = gp

	if se, ok := gp.(Executable); ok {
		w.e = se
	}
	if m, ok := gp.(Matcher); ok {
		w.m = m
	}
	if s, ok := gp.(Service); ok {
		w.s = s
	}

	return w
}

func (w *PluginWrapper) GetPlugin() Plugin {
	return w.p
}

func (w *PluginWrapper) Match(ctx context.Context, qCtx *Context) (matched bool, err error) {
	matched, err = w.match(ctx, qCtx)
	if err != nil {
		return false, NewPluginError(w.p.Tag(), err)
	}
	mlog.L().Debug("matching query context", qCtx.InfoField(), zap.String("tag", w.p.Tag()), zap.Bool("result", matched))
	return matched, nil
}

func (w *PluginWrapper) match(ctx context.Context, qCtx *Context) (matched bool, err error) {
	if err = ctx.Err(); err != nil {
		return false, err
	}

	if w.m == nil {
		return false, fmt.Errorf("plugin tag: %s, type: %s is not a Matcher", w.p.Tag(), w.p.Type())
	}

	return w.m.Match(ctx, qCtx)
}

func (w *PluginWrapper) Exec(ctx context.Context, qCtx *Context, next ExecutableChainNode) error {
	mlog.L().Debug("executing plugin", qCtx.InfoField(), zap.String("tag", w.p.Tag()))
	err := w.exec(ctx, qCtx, next)
	if err != nil {
		return NewPluginError(w.p.Tag(), err)
	}
	return nil
}

func (w *PluginWrapper) exec(ctx context.Context, qCtx *Context, next ExecutableChainNode) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if w.e == nil {
		return fmt.Errorf("plugin tag: %s, type: %s is not an ESExecutable nor Executable", w.p.Tag(), w.p.Type())
	}

	return w.e.Exec(ctx, qCtx, next)
}

func (w *PluginWrapper) Shutdown() error {
	mlog.L().Debug("shutting down service", zap.String("tag", w.p.Tag()))

	if w.s == nil {
		return fmt.Errorf("plugin tag: %s, type: %s is not a Service", w.p.Tag(), w.p.Type())
	}
	err := w.s.Shutdown()
	if err != nil {
		return NewPluginError(w.p.Tag(), err)
	}
	return nil
}

type PluginInterfaceType uint8

const (
	PITESExecutable = iota
	PITMatcher
	PITService
)

func (w *PluginWrapper) Is(t PluginInterfaceType) bool {
	switch t {
	case PITESExecutable:
		return w.e != nil
	case PITMatcher:
		return w.m != nil
	case PITService:
		return w.s != nil
	default:
		panic(fmt.Sprintf("hander: invalid PluginInterfaceType: %d", t))
	}
}
