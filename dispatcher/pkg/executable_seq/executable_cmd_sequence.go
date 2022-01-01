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
	"github.com/IrineSistiana/mosdns/v2/dispatcher/handler"
	"go.uber.org/zap"
	"reflect"
)

// NewREPNode creates a RefExecPluginNode from tag.
func NewREPNode(tag string) RefExecPluginNode {
	return RefExecPluginNode(tag)
}

// RefExecPluginNode is a handler.ExecutablePlugin reference tag.
type RefExecPluginNode string

func (n RefExecPluginNode) Exec(ctx context.Context, qCtx *handler.Context, next handler.ExecutableChainNode) error {
	p := handler.GetPlugin(string(n))
	if p == nil {
		return fmt.Errorf("plugin [%s] is not registered", string(n))
	}
	return p.Exec(ctx, qCtx, next)
}

// RefMatcherPluginNode is a handler.MatcherPlugin reference tag.
type RefMatcherPluginNode string

func (n RefMatcherPluginNode) Match(ctx context.Context, qCtx *handler.Context) (bool, error) {
	p := handler.GetPlugin(string(n))
	if p == nil {
		return false, fmt.Errorf("plugin [%s] is not registered", n)
	}
	return p.Match(ctx, qCtx)
}

// ParseExecutableNode parses in into a ExecutableChainNode.
// in can be: (a / a slice of) Executable,
// (a / a slice of) string of registered handler.ExecutablePlugin tag,
// (a / a slice of) map[string]interface{}, which can be parsed to FallbackConfig, ParallelConfig or IfNodeConfig,
// a []interface{} that contains all the above.
func ParseExecutableNode(in interface{}, logger *zap.Logger) (handler.ExecutableChainNode, error) {
	switch v := in.(type) {
	case handler.ExecutableChainNode:
		return v, nil

	case handler.Executable:
		return handler.WrapExecutable(v), nil

	case []interface{}:
		var rootNode handler.ExecutableChainNode
		var tailNode handler.ExecutableChainNode
		for i, elem := range v {
			n, err := ParseExecutableNode(elem, logger)
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
		return handler.WrapExecutable(NewREPNode(v)), nil

	case map[string]interface{}:
		switch {
		case hasKey(v, "if") || hasKey(v, "if_and"): // if block
			ec, err := parseIfBlockFromMap(v, logger)
			if err != nil {
				return nil, fmt.Errorf("invalid if section: %w", err)
			}
			return ec, nil
		case hasKey(v, "parallel"): // parallel
			ec, err := parseParallelNodeFromMap(v, logger)
			if err != nil {
				return nil, fmt.Errorf("invalid parallel section: %w", err)
			}
			return ec, nil
		case hasKey(v, "load_balance"): // load balance
			ec, err := parseLBNodeFromMap(v, logger)
			if err != nil {
				return nil, fmt.Errorf("invalid load balance section: %w", err)
			}
			return ec, nil
		case hasKey(v, "primary") || hasKey(v, "secondary"): // fallback
			ec, err := parseFallbackNodeFromMap(v, logger)
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

func parseIfBlockFromMap(m map[string]interface{}, logger *zap.Logger) (handler.ExecutableChainNode, error) {
	conf := new(IfNodeConfig)
	err := handler.WeakDecode(m, conf)
	if err != nil {
		return nil, err
	}

	e, err := ParseIfChainNode(conf, logger)
	if err != nil {
		return nil, err
	}

	return e, nil
}

func parseParallelNodeFromMap(m map[string]interface{}, logger *zap.Logger) (handler.ExecutableChainNode, error) {
	conf := new(ParallelConfig)
	err := handler.WeakDecode(m, conf)
	if err != nil {
		return nil, err
	}
	e, err := ParseParallelNode(conf, logger)
	if err != nil {
		return nil, err
	}

	return handler.WrapExecutable(e), nil
}

func parseFallbackNodeFromMap(m map[string]interface{}, logger *zap.Logger) (handler.ExecutableChainNode, error) {
	conf := new(FallbackConfig)
	err := handler.WeakDecode(m, conf)
	if err != nil {
		return nil, err
	}
	e, err := ParseFallbackNode(conf, logger)
	if err != nil {
		return nil, err
	}

	return handler.WrapExecutable(e), nil
}

func parseLBNodeFromMap(m map[string]interface{}, logger *zap.Logger) (handler.ExecutableChainNode, error) {
	conf := new(LBConfig)
	err := handler.WeakDecode(m, conf)
	if err != nil {
		return nil, err
	}
	e, err := ParseLBNode(conf, logger)
	if err != nil {
		return nil, err
	}

	return e, nil
}

func hasKey(m map[string]interface{}, key string) bool {
	_, ok := m[key]
	return ok
}
