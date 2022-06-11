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
		var rootNode ExecutableChainNode
		var tailNode ExecutableChainNode
		for i, elem := range v {
			n, err := BuildExecutableLogicTree(elem, logger, execs, matchers)
			if err != nil {
				return nil, fmt.Errorf("invalid cmd at #%d: %w", i, err)
			}

			if rootNode == nil {
				rootNode = n
			}
			if tailNode != nil {
				tailNode.LinkNext(n)
			}
			tailNode = n
		}
		return rootNode, nil

	case string:
		exec := execs[v]
		if exec == nil {
			return nil, fmt.Errorf("can not find execuable %s", v)
		}
		return WrapExecutable(exec), nil

	case map[string]interface{}:
		switch {
		case hasKey(v, "if") || hasKey(v, "if_and"): // if block
			ec, err := parseIfBlockFromMap(v, logger, execs, matchers)
			if err != nil {
				return nil, fmt.Errorf("invalid if section: %w", err)
			}
			return ec, nil
		case hasKey(v, "parallel"): // parallel
			ec, err := parseParallelNodeFromMap(v, logger, execs, matchers)
			if err != nil {
				return nil, fmt.Errorf("invalid parallel section: %w", err)
			}
			return ec, nil
		case hasKey(v, "load_balance"): // load balance
			ec, err := parseLBNodeFromMap(v, logger, execs, matchers)
			if err != nil {
				return nil, fmt.Errorf("invalid load balance section: %w", err)
			}
			return ec, nil
		case hasKey(v, "primary") || hasKey(v, "secondary"): // fallback
			ec, err := parseFallbackNodeFromMap(v, logger, execs, matchers)
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

func parseIfBlockFromMap(
	m map[string]interface{},
	logger *zap.Logger,
	execs map[string]Executable,
	matchers map[string]Matcher,
) (ExecutableChainNode, error) {
	conf := new(ConditionNodeConfig)
	err := utils.WeakDecode(m, conf)
	if err != nil {
		return nil, err
	}

	e, err := ParseConditionNode(conf, logger, execs, matchers)
	if err != nil {
		return nil, err
	}

	return e, nil
}

func parseParallelNodeFromMap(
	m map[string]interface{},
	logger *zap.Logger,
	execs map[string]Executable,
	matchers map[string]Matcher,
) (ExecutableChainNode, error) {
	conf := new(ParallelConfig)
	err := utils.WeakDecode(m, conf)
	if err != nil {
		return nil, err
	}
	e, err := ParseParallelNode(conf, logger, execs, matchers)
	if err != nil {
		return nil, err
	}

	return WrapExecutable(e), nil
}

func parseFallbackNodeFromMap(
	m map[string]interface{},
	logger *zap.Logger,
	execs map[string]Executable,
	matchers map[string]Matcher,
) (ExecutableChainNode, error) {
	conf := new(FallbackConfig)
	err := utils.WeakDecode(m, conf)
	if err != nil {
		return nil, err
	}
	e, err := ParseFallbackNode(conf, logger, execs, matchers)
	if err != nil {
		return nil, err
	}

	return WrapExecutable(e), nil
}

func parseLBNodeFromMap(
	m map[string]interface{},
	logger *zap.Logger,
	execs map[string]Executable,
	matchers map[string]Matcher,
) (ExecutableChainNode, error) {
	conf := new(LBConfig)
	err := utils.WeakDecode(m, conf)
	if err != nil {
		return nil, err
	}
	e, err := ParseLBNode(conf, logger, execs, matchers)
	if err != nil {
		return nil, err
	}

	return e, nil
}

func hasKey(m map[string]interface{}, key string) bool {
	_, ok := m[key]
	return ok
}
