/*
 * Copyright (C) 2020-2022, IrineSistiana
 *
 * This file is part of mosdns.
 *
 * mosdns is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * mosdns is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

package executable_seq

import (
	"errors"
	"fmt"
	"github.com/IrineSistiana/mosdns/v4/pkg/utils"
	"go.uber.org/zap"
	"reflect"
	"strconv"
)

// BuildExecutableLogicTree parses in into a ExecutableChainNode.
// in can be: (a / a slice of) Executable,
// (a / a slice of) string that map to an Executable in execs,
// (a / a slice of) map[string]interface{}, which can be parsed to FallbackConfig, ParallelConfig or ConditionNodeConfig,
// a []interface{} that contains all the above.
func BuildExecutableLogicTree(
	in interface{},
	logger *zap.Logger,
	execs map[string]Executable,
	matchers map[string]Matcher,
) (ExecutableChainNode, error) {
	switch v := in.(type) {
	case ExecutableChainNode:
		return v, nil

	case Executable:
		return WrapExecutable(v), nil

	case []interface{}:
		nodes := make([]ExecutableChainNode, len(v))
		for i, elem := range v {
			nodeLogger := logger.Named(fmt.Sprintf("node_%d", i))
			n, err := BuildExecutableLogicTree(elem, nodeLogger, execs, matchers)
			if err != nil {
				return nil, fmt.Errorf("invalid cmd at #%d: %w", i, err)
			}
			nodes[i] = n
		}
		return linkNodes(nodes), nil

	case string:
		exec := execs[v]
		if exec == nil {
			return nil, fmt.Errorf("can not find executable %s", v)
		}
		return WrapExecutable(exec), nil

	case map[string]interface{}:
		switch {
		case hasKey(v, "if") || hasKey(v, "if_and"): // if block
			return parseIfBlockFromMap(v, logger, execs, matchers)
		case hasKey(v, "parallel"): // parallel
			return parseParallelNodeFromMap(v, logger, execs, matchers)
		case hasKey(v, "load_balance"): // load balance
			return parseLBNodeFromMap(v, logger, execs, matchers)
		case hasKey(v, "primary") || hasKey(v, "secondary"): // fallback
			return parseFallbackNodeFromMap(v, logger, execs, matchers)
		default:
			return nil, errors.New("unknown section")
		}
	default:
		return nil, fmt.Errorf("unexpected type: %s", reflect.TypeOf(in).String())
	}
}

func linkNodes(nodes []ExecutableChainNode) ExecutableChainNode {
	if len(nodes) == 0 {
		return nil
	}
	for i := 0; i < len(nodes)-1; i++ {
		nodes[i].LinkNext(nodes[i+1])
	}
	return nodes[0]
}

func parseIfBlockFromMap(
	m map[string]interface{},
	logger *zap.Logger,
	execs map[string]Executable,
	matchers map[string]Matcher,
) (ExecutableChainNode, error) {
	conf := new(ConditionNodeConfig)
	if err := utils.WeakDecode(m, conf); err != nil {
		return nil, err
	}
	return ParseConditionNode(conf, logger, execs, matchers)
}

func parseParallelNodeFromMap(
	m map[string]interface{},
	logger *zap.Logger,
	execs map[string]Executable,
	matchers map[string]Matcher,
) (ExecutableChainNode, error) {
	conf := new(ParallelConfig)
	if err := utils.WeakDecode(m, conf); err != nil {
		return nil, err
	}
	return WrapExecutable(ParseParallelNode(conf, logger, execs, matchers))
}

func parseFallbackNodeFromMap(
	m map[string]interface{},
	logger *zap.Logger,
	execs map[string]Executable,
	matchers map[string]Matcher,
) (ExecutableChainNode, error) {
	conf := new(FallbackConfig)
	if err := utils.WeakDecode(m, conf); err != nil {
		return nil, err
	}
	return WrapExecutable(ParseFallbackNode(conf, logger, execs, matchers))
}

func parseLBNodeFromMap(
	m map[string]interface{},
	logger *zap.Logger,
	execs map[string]Executable,
	matchers map[string]Matcher,
) (ExecutableChainNode, error) {
	conf := new(LBConfig)
	if err := utils.WeakDecode(m, conf); err != nil {
		return nil, err
	}
	return ParseLBNode(conf, logger, execs, matchers)
}

func hasKey(m map[string]interface{}, key string) bool {
	_, ok := m[key]
	return ok
}