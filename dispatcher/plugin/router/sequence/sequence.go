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
	"fmt"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/dispatcher/mlog"
	"github.com/sirupsen/logrus"
	"reflect"
	"strings"
)

const PluginType = "sequence"

func init() {
	handler.RegInitFunc(PluginType, Init)

	handler.MustRegPlugin(&sequencePlugin{tag: "_end"})
}

var _ handler.RouterPlugin = (*sequencePlugin)(nil)

type sequencePlugin struct {
	tag        string
	executable []executable
	next       string

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

	executable := make([]executable, 0, len(args.Exec))
	if err := parse(args.Exec, &executable); err != nil {
		return nil, handler.NewErrFromTemplate(handler.ETInvalidArgs, err)
	}

	s := newSequencePlugin(tag, executable, args.Next)
	return s, nil
}

func (s *sequencePlugin) Tag() string {
	return s.tag
}

func (s *sequencePlugin) Type() string {
	return PluginType
}

func (s *sequencePlugin) Do(ctx context.Context, qCtx *handler.Context) (next string, err error) {
	goTwo, err := walk(ctx, qCtx, s.executable, s.logger)
	if err != nil {
		return "", err
	}
	if len(goTwo) != 0 {
		return goTwo, nil
	}
	return s.next, nil
}

func newSequencePlugin(tag string, executable []executable, next string) *sequencePlugin {
	return &sequencePlugin{tag: tag, executable: executable, next: next, logger: mlog.NewPluginLogger(tag)}
}

type executable interface {
	exec(ctx context.Context, qCtx *handler.Context, logger *logrus.Entry) (goTwo string, err error)
}

type functionalPlugin string

func (tag functionalPlugin) exec(ctx context.Context, qCtx *handler.Context, logger *logrus.Entry) (goTwo string, err error) {
	if len(tag) == 0 {
		return "", nil
	}
	logger.Debugf("%v: exec functional plugin %s", qCtx, tag)

	p, err := handler.GetFunctionalPlugin(string(tag))
	if err != nil {
		return "", err
	}
	return "", p.Do(ctx, qCtx)
}

type IfBlockConfig struct {
	If   []string      `yaml:"if"`
	Exec []interface{} `yaml:"exec"`
	Goto string        `yaml:"goto"`
}

type ifBlock struct {
	ifMather   []string
	executable []executable
	gotoRouter string
}

func (b *ifBlock) exec(ctx context.Context, qCtx *handler.Context, logger *logrus.Entry) (goTwo string, err error) {
	If := true
	for _, tag := range b.ifMather {
		if len(tag) == 0 {
			continue
		}

		reverse := false
		if reverse = strings.HasPrefix(tag, "!"); reverse {
			tag = strings.TrimPrefix(tag, "!")
		}

		m, err := handler.GetMatcherPlugin(tag)
		if err != nil {
			return "", err
		}
		matched, err := m.Match(ctx, qCtx)
		if err != nil {
			return "", handler.NewErrFromTemplate(handler.ETPluginErr, tag, err)
		}
		logger.Debugf("%v: exec matcher plugin %s, returned: %v", qCtx, tag, matched)

		If = matched != reverse
		if If == true {
			break // if one of the case is true, skip others.
		}
	}
	if If == false {
		return "", nil // if case returns false, skip this block.
	}

	// exec
	goTwo, err = walk(ctx, qCtx, b.executable, logger) // exec its sub exec sequence
	if err != nil {
		return "", err
	}
	if len(goTwo) != 0 {
		return goTwo, nil
	}

	// goto
	if len(b.gotoRouter) != 0 { // if block has a goto, return it
		return b.gotoRouter, nil
	}

	return "", nil
}

// parse parses []interface{} to []executable
func parse(in []interface{}, out *[]executable) error {
	for i := range in {
		switch v := in[i].(type) {
		case string:
			*out = append(*out, functionalPlugin(v))
		case map[string]interface{}:
			c := new(IfBlockConfig)
			err := handler.WeakDecode(v, c)
			if err != nil {
				return err
			}

			ifBlock := &ifBlock{
				ifMather:   c.If,
				executable: make([]executable, 0, len(c.Exec)),
				gotoRouter: c.Goto,
			}

			if len(c.Exec) != 0 {
				err := parse(c.Exec, &ifBlock.executable)
				if err != nil {
					return err
				}
			}
			*out = append(*out, ifBlock)
		default:
			return fmt.Errorf("unexpected type: %s", reflect.TypeOf(in[i]).String())
		}
	}
	return nil
}

func walk(ctx context.Context, qCtx *handler.Context, sequence []executable, logger *logrus.Entry) (next string, err error) {
	for _, e := range sequence {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		goTwo, err := e.exec(ctx, qCtx, logger)
		if err != nil {
			if tag, ok := e.(functionalPlugin); ok {
				return "", handler.NewErrFromTemplate(handler.ETPluginErr, tag, err)
			}
			return "", err
		}
		if len(goTwo) != 0 {
			return goTwo, nil
		}
	}

	return "", nil
}
