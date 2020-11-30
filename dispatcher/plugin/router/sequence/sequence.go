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
	"github.com/sirupsen/logrus"
	"strings"
)

const PluginType = "sequence"

func init() {
	handler.RegInitFunc(PluginType, Init)
	handler.SetTemArgs(PluginType, &Args{Sequence: []*Block{
		{
			If:       []string{"", ""},
			Exec:     []string{"", ""},
			Sequence: nil,
			Goto:     "",
		},
	}})
}

var _ handler.RouterPlugin = (*sequence)(nil)

type sequence struct {
	tag  string
	args *Args
}

type Args struct {
	Sequence []*Block `yaml:"sequence"`
	Next     string   `yaml:"next"`
}

type Block struct {
	If       []string `yaml:"if"`
	Exec     []string `yaml:"exec"`
	Sequence []*Block `yaml:"sequence"`
	Goto     string   `yaml:"goto"`
}

func walk(ctx context.Context, qCtx *handler.Context, i []*Block) (next string, err error) {
	for _, block := range i {

		// if
		If := true
		for _, tag := range block.If {
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
		for _, tag := range block.Exec {
			if len(tag) == 0 {
				continue
			}
			err = getPluginAndExec(ctx, qCtx, tag)
			if err != nil {
				return "", handler.NewErrFromTemplate(handler.ETPluginErr, block.Exec, err)
			}
		}

		// sequence
		next, err = walk(ctx, qCtx, block.Sequence) // exec its sub block
		if err != nil {
			return "", err
		}
		if len(next) != 0 {
			return next, nil
		}

		// goto
		if len(block.Goto) != 0 { // if block has a goto, return it
			return block.Goto, nil
		}
	}

	return "", nil
}

func Init(tag string, argsMap handler.Args) (p handler.Plugin, err error) {
	args := new(Args)
	err = argsMap.WeakDecode(args)
	if err != nil {
		return nil, handler.NewErrFromTemplate(handler.ETInvalidArgs, err)
	}

	if len(args.Sequence) == 0 {
		return nil, errors.New("empty exec sequence")
	}

	s := new(sequence)
	s.tag = tag
	s.args = args

	return s, nil
}

func (s *sequence) Tag() string {
	return s.tag
}

func (s *sequence) Type() string {
	return PluginType
}

func (s *sequence) Do(ctx context.Context, qCtx *handler.Context) (next string, err error) {
	next, err = walk(ctx, qCtx, s.args.Sequence)
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
