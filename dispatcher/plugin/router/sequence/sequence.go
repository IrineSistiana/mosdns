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
	"github.com/IrineSistiana/mosdns/dispatcher/logger"
	"github.com/sirupsen/logrus"
	"reflect"
	"strings"
)

const PluginType = "sequence"

func init() {
	handler.RegInitFunc(PluginType, Init)
	handler.SetTemArgs(PluginType, &Args{Exec: []interface{}{"", "",
		&IfBlock{
			If:   []string{"", ""},
			Exec: nil,
			Goto: "",
		},
	}})
}

var _ handler.RouterPlugin = (*sequencePlugin)(nil)

type sequencePlugin struct {
	tag  string
	args *Args
}

// parse parses map[string]interface{} to IfBlock
func parse(s *[]interface{}) error {
	for i, e := range *s {
		switch v := e.(type) {
		case string:
		case []interface{}:
			err := parse(&v)
			if err != nil {
				return err
			}
		case map[string]interface{}:
			ifBlock := new(IfBlock)
			err := handler.WeakDecode(v, ifBlock)
			if err != nil {
				return err
			}
			(*s)[i] = ifBlock
		default:
			return fmt.Errorf("unexpected type: %s", reflect.TypeOf(e).Name())
		}
	}
	return nil
}

type Args struct {
	Exec []interface{} `yaml:"exec"`
	Next string        `yaml:"next"`
}

type IfBlock struct {
	If   []string      `yaml:"if"`
	Exec []interface{} `yaml:"exec"`
	Goto string        `yaml:"goto"`
}

func walk(ctx context.Context, qCtx *handler.Context, sequence []interface{}) (next string, err error) {
	for _, i := range sequence {
		if err := ctx.Err(); err != nil {
			return "", err
		}

		switch e := i.(type) {
		case string: // is a tag
			if len(e) != 0 {
				err := getPluginAndExec(ctx, qCtx, e)
				if err != nil {
					return "", handler.NewErrFromTemplate(handler.ETPluginErr, e, err)
				}
			}

		case *IfBlock: // is a if block
			// if
			If := true
			for _, tag := range e.If {
				if len(tag) == 0 {
					continue
				}
				reverse := false
				if reverse = strings.HasPrefix(tag, "!"); reverse {
					tag = strings.TrimPrefix(tag, "!")
				}
				matched, err := getPluginAndMatch(ctx, qCtx, tag)
				if err != nil {
					return "", handler.NewErrFromTemplate(handler.ETPluginErr, tag, err)
				}

				If = matched != reverse
				if If == true {
					break // if one of the case is true, skip others.
				}
			}
			if If == false {
				continue // if case returns false, skip this block.
			}

			// exec
			next, err = walk(ctx, qCtx, e.Exec) // exec its sub exec sequence
			if err != nil {
				return "", err
			}
			if len(next) != 0 {
				return next, nil
			}

			// goto
			if len(e.Goto) != 0 { // if block has a goto, return it
				return e.Goto, nil
			}

		default:
			logger.Entry().Warnf("internal err: unexpected sequence element type: %s", reflect.TypeOf(e).Name())
		}

	}

	return "", nil
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

	if err := parse(&args.Exec); err != nil {
		return nil, handler.NewErrFromTemplate(handler.ETInvalidArgs, err)
	}

	s := new(sequencePlugin)
	s.tag = tag
	s.args = args

	return s, nil
}

func (s *sequencePlugin) Tag() string {
	return s.tag
}

func (s *sequencePlugin) Type() string {
	return PluginType
}

func (s *sequencePlugin) Do(ctx context.Context, qCtx *handler.Context) (next string, err error) {
	next, err = walk(ctx, qCtx, s.args.Exec)
	if err != nil {
		return "", err
	}
	if len(next) != 0 {
		return next, nil
	}
	return s.args.Next, nil
}

func getPluginAndExec(ctx context.Context, qCtx *handler.Context, tag string) (err error) {
	qCtx.Logf(logrus.DebugLevel, "exec plugin %s", tag)
	p, ok := handler.GetFunctionalPlugin(tag)
	if !ok {
		return handler.NewErrFromTemplate(handler.ETTagNotDefined, tag)
	}
	return p.Do(ctx, qCtx)
}

func getPluginAndMatch(ctx context.Context, qCtx *handler.Context, tag string) (ok bool, err error) {
	qCtx.Logf(logrus.DebugLevel, "exec plugin %s", tag)
	m, ok := handler.GetMatcherPlugin(tag)
	if !ok {
		return false, handler.NewErrFromTemplate(handler.ETTagNotDefined, tag)
	}

	return m.Match(ctx, qCtx)
}
