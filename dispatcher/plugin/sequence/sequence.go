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
)

func init() {
	handler.RegInitFunc("sequence", Init)
}

type sequence struct {
	tags []string
}

type Args struct {
	Sequence []string `yaml:"sequence"`
	Next     string   `yaml:"next"`
}

func Init(conf *handler.Config) (p handler.Plugin, err error) {
	args := new(Args)
	err = conf.Args.WeakDecode(args)
	if err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}

	if len(args.Sequence) == 0 {
		return nil, errors.New("emtpy sequence")
	}

	b := new(sequence)
	b.tags = args.Sequence

	return handler.WrapOneWayPlugin(conf, b, args.Next), nil
}

func (s *sequence) Modify(ctx context.Context, qCtx *handler.Context) (err error) {
	for _, tag := range s.tags {
		p, ok := handler.GetPlugin(tag)
		if !ok {
			return handler.NewTagNotDefinedErr(tag)
		}

		_, err = p.Do(ctx, qCtx)
		if err != nil {
			return fmt.Errorf("plugin %s reported an err: %w", tag, err)
		}
	}
	return nil
}
