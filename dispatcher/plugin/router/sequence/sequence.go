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
	"strings"
)

const PluginType = "sequence"

func init() {
	handler.RegInitFunc(PluginType, Init)
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
	If       string   `yaml:"if"`
	Exec     []string `yaml:"exec"`
	Sequence []*Block `yaml:"sequence"`
	Goto     string   `yaml:"goto"`
}

func walk(ctx context.Context, qCtx *handler.Context, i []*Block) (next string, err error) {
	for _, block := range i {
		if len(block.If) != 0 {
			ifTag := block.If
			reverse := false
			if strings.HasPrefix(ifTag, "!") {
				reverse = true
				ifTag = strings.TrimPrefix(ifTag, "!")
			}
			ok, err := getPluginAndMatch(ctx, qCtx, ifTag)
			if err != nil {
				return "", fmt.Errorf("plugin %s reported an err: %w", block.If, err)
			}
			if ok == reverse { // block has an if case but returns false. Skip this block.
				continue
			}
		}

		if len(block.Exec) != 0 {
			for _, tag := range block.Exec {
				err = getPluginAndExec(ctx, qCtx, tag)
				if err != nil {
					return "", fmt.Errorf("plugin %s reported an err: %w", block.Exec, err)
				}
			}
		}

		next, err = walk(ctx, qCtx, block.Sequence) // exec its sub block
		if err != nil {
			return "", err
		}
		if len(next) != 0 {
			return next, nil
		}

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
		return nil, fmt.Errorf("invalid args: %w", err)
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
	logger.GetStd().Debugf("%v: exec plugin %s", qCtx, tag)
	p, ok := handler.GetFunctionalPlugin(tag)
	if !ok {
		return handler.NewTagNotDefinedErr(tag)
	}
	return p.Do(ctx, qCtx)
}

func getPluginAndMatch(ctx context.Context, qCtx *handler.Context, tag string) (ok bool, err error) {
	logger.GetStd().Debugf("%v: exec plugin %s", qCtx, tag)
	m, ok := handler.GetMatcherPlugin(tag)
	if !ok {
		return false, handler.NewTagNotDefinedErr(tag)
	}

	return m.Match(ctx, qCtx)
}
