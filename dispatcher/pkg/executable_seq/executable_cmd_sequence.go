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

package executable_seq

import (
	"context"
	"errors"
	"fmt"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"go.uber.org/zap"
	"reflect"
)

// EarlyStop is a noop ExecutableNode that returns earlyStop == true.
var EarlyStop earlyStop

type earlyStop struct {
	LinkedListElem
}

func (e *earlyStop) Exec(_ context.Context, _ *handler.Context, _ *zap.Logger) (earlyStop bool, err error) {
	return true, nil
}

// NewRESEPNode creates a RefESExecutablePluginNode from tag.
func NewRESEPNode(tag string) *RefESExecutablePluginNode {
	return &RefESExecutablePluginNode{ref: tag}
}

// RefESExecutablePluginNode is a handler.ESExecutablePlugin reference tag.
type RefESExecutablePluginNode struct {
	ref string
	LinkedListElem
}

func (n *RefESExecutablePluginNode) Exec(ctx context.Context, qCtx *handler.Context, _ *zap.Logger) (earlyStop bool, err error) {
	p, err := handler.GetPlugin(n.ref)
	if err != nil {
		return false, err
	}

	return p.ExecES(ctx, qCtx)
}

// RefMatcherPluginNode is a handler.MatcherPlugin reference tag.
type RefMatcherPluginNode string

func (ref RefMatcherPluginNode) Match(ctx context.Context, qCtx *handler.Context) (matched bool, err error) {
	p, err := handler.GetPlugin(string(ref))
	if err != nil {
		return false, err
	}
	return p.Match(ctx, qCtx)
}

// ParseExecutableNode parses in into a ExecutableNode.
// in can be: (a / a slice of) Executable,
// (a / a slice of) string of registered handler.ESExecutablePlugin tag,
// (a / a slice of) map[string]interface{}, which can be parsed to FallbackConfig, ParallelECSConfig or IfBlockConfig,
// a []interface{} that contains all of the above.
func ParseExecutableNode(in interface{}) (ExecutableNode, error) {
	switch v := in.(type) {
	case Executable:
		return WarpExecutable(v), nil

	case []interface{}:
		var rootNode ExecutableNode
		var tailNode ExecutableNode
		for i, elem := range v {
			n, err := ParseExecutableNode(elem)
			if err != nil {
				return nil, fmt.Errorf("invalid cmd at #%d: %w", i, err)
			}

			if rootNode == nil {
				rootNode = n
			}
			if tailNode != nil {
				tailNode.LinkNext(n)
				n.LinkPrevious(tailNode)
			}
			tailNode = n
		}
		return rootNode, nil

	case string:
		return NewRESEPNode(v), nil

	case map[string]interface{}:
		switch {
		case hasKey(v, "if") || hasKey(v, "if_and"): // if block
			ec, err := parseIfBlockFromMap(v)
			if err != nil {
				return nil, fmt.Errorf("invalid if section: %w", err)
			}
			return ec, nil
		case hasKey(v, "parallel"): // parallel
			ec, err := parseParallelECSFromMap(v)
			if err != nil {
				return nil, fmt.Errorf("invalid parallel section: %w", err)
			}
			return ec, nil
		case hasKey(v, "primary") || hasKey(v, "secondary"): // fallback
			ec, err := parseFallbackECSFromMap(v)
			if err != nil {
				return nil, fmt.Errorf("invalid fallback section: %w", err)
			}
			return ec, nil
		default:
			return nil, errors.New("unknown section")
		}
	default:
		return nil, fmt.Errorf("unexpected type: %s", reflect.TypeOf(in).String())
	}
}

func parseIfBlockFromMap(m map[string]interface{}) (ExecutableNode, error) {
	conf := new(IfBlockConfig)
	err := handler.WeakDecode(m, conf)
	if err != nil {
		return nil, err
	}

	e, err := ParseIfBlock(conf)
	if err != nil {
		return nil, err
	}

	return WarpExecutable(e), nil
}

func parseParallelECSFromMap(m map[string]interface{}) (ExecutableNode, error) {
	conf := new(ParallelECSConfig)
	err := handler.WeakDecode(m, conf)
	if err != nil {
		return nil, err
	}
	e, err := ParseParallelECS(conf)
	if err != nil {
		return nil, err
	}

	return WarpExecutable(e), nil
}

func parseFallbackECSFromMap(m map[string]interface{}) (ExecutableNode, error) {
	conf := new(FallbackConfig)
	err := handler.WeakDecode(m, conf)
	if err != nil {
		return nil, err
	}
	e, err := ParseFallbackECS(conf)
	if err != nil {
		return nil, err
	}

	return WarpExecutable(e), nil
}

func hasKey(m map[string]interface{}, key string) bool {
	_, ok := m[key]
	return ok
}
