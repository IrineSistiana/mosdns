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

package domain

import (
	"bufio"
	"errors"
	"fmt"
	"github.com/IrineSistiana/mosdns/v5/pkg/utils"
	"io"
	"strings"
	"unicode"
)

// ParseStringFunc parse data string to matcher pattern and additional attributions.
type ParseStringFunc[T any] func(s string) (pattern string, v T, err error)

// check if s only contains a domain pattern (no other section, no space).
func patternOnly[T any](s string) (pattern string, v T, err error) {
	if strings.IndexFunc(s, unicode.IsSpace) != -1 {
		return "", v, errors.New("rule string has more than one section")
	}
	return s, v, nil
}

// Load loads data from a string, parsing it with parseString function.
func Load[T any](m WriteableMatcher[T], s string, parseString ParseStringFunc[T]) error {
	if parseString == nil {
		parseString = patternOnly[T]
	}
	pattern, v, err := parseString(s)
	if err != nil {
		return err
	}
	return m.Add(pattern, v)
}

// LoadFromTextReader loads multiple lines from reader r. r
func LoadFromTextReader[T any](m WriteableMatcher[T], r io.Reader, parseString ParseStringFunc[T]) error {
	lineCounter := 0
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		lineCounter++
		s := scanner.Text()
		s = utils.RemoveComment(s, "#")
		s = strings.TrimSpace(s)
		if len(s) == 0 {
			continue
		}

		err := Load(m, s, parseString)
		if err != nil {
			return fmt.Errorf("line %d: %v", lineCounter, err)
		}
	}
	return scanner.Err()
}

func NewDomainMixMatcher() *MixMatcher[struct{}] {
	mixMatcher := NewMixMatcher[struct{}]()
	mixMatcher.SetDefaultMatcher(MatcherDomain)
	return mixMatcher
}
