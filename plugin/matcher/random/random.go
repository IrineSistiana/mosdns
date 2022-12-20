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

package env

import (
	"context"
	"errors"
	"fmt"
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
	"math/rand"
	"strconv"
	"sync"
	"time"
)

const PluginType = "random"

func init() {
	sequence.MustRegMatchQuickSetup(PluginType, QuickSetup)
}

func QuickSetup(_ sequence.BQ, s string) (sequence.Matcher, error) {
	if len(s) == 0 {
		return nil, errors.New("a float64 probability is required")
	}
	p, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid probability, %w", err)
	}
	return Random(p), nil
}

type random struct {
	prob float64

	m sync.Mutex
	r *rand.Rand
}

func (r *random) Match(_ context.Context, _ *query_context.Context) (bool, error) {
	return r.RandBool(), nil
}

func (r *random) RandBool() bool {
	r.m.Lock()
	defer r.m.Unlock()
	return r.r.Float64() < r.prob
}

// Random returns a sequence.Matcher that returns true with a probability of prob.
func Random(prob float64) sequence.Matcher {
	return &random{prob: prob, r: rand.New(rand.NewSource(time.Now().UnixNano()))}
}
