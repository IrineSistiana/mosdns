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

package sequence

import (
	"context"
	"errors"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/dispatcher/mlog"
	"github.com/sirupsen/logrus"
)

const PluginType = "sequence"

func init() {
	handler.RegInitFunc(PluginType, Init)

	handler.MustRegPlugin(&sequenceRouter{tag: "_end"})
}

var _ handler.ExecutablePlugin = (*sequenceRouter)(nil)

type sequenceRouter struct {
	tag           string
	executableCmd *handler.ExecutableCmdSequence
	next          string

	logger *logrus.Entry
}

type Args struct {
	Exec []interface{} `yaml:"exec"`
	Next string        `yaml:"next"`
}

func Init(tag string, argsMap map[string]interface{}) (p handler.Plugin, err error) {
	args := new(Args)
	err = handler.WeakDecode(argsMap, args)
	if err != nil {
		return nil, handler.NewErrFromTemplate(handler.ETInvalidArgs, err)
	}

	if len(args.Exec) == 0 {
		return nil, errors.New("empty exec sequence")
	}

	ecs := handler.NewExecutableCmdSequence()
	if err := ecs.Parse(args.Exec); err != nil {
		return nil, handler.NewErrFromTemplate(handler.ETInvalidArgs, err)
	}

	s := newSequencePlugin(tag, ecs, args.Next)
	return s, nil
}

func newSequencePlugin(tag string, executable *handler.ExecutableCmdSequence, next string) *sequenceRouter {
	return &sequenceRouter{tag: tag, executableCmd: executable, next: next, logger: mlog.NewPluginLogger(tag)}
}

func (s *sequenceRouter) Tag() string {
	return s.tag
}

func (s *sequenceRouter) Type() string {
	return PluginType
}

func (s *sequenceRouter) Exec(ctx context.Context, qCtx *handler.Context) (err error) {
	if s.executableCmd != nil {
		err = s.executableCmd.Exec(ctx, qCtx, s.logger)
		if err != nil {
			return handler.NewPluginError(s.tag, err)
		}
	}
	return nil
}
