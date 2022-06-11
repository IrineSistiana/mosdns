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

import "strings"

type DomainScanner struct {
	d string
	n int
}

func NewUnifiedDomainScanner(s string) *DomainScanner {
	domain := UnifyDomain(s)
	return &DomainScanner{
		d: domain,
		n: len(domain),
	}
}

func (s *DomainScanner) Scan() bool {
	return s.n > 0
}

func (s *DomainScanner) PrevLabelOffset() int {
	s.n = strings.LastIndexByte(s.d[:s.n], '.')
	return s.n + 1
}

func (s *DomainScanner) PrevLabel() (label string, end bool) {
	n := strings.LastIndexByte(s.d[:s.n], '.')
	l := s.d[n+1 : s.n]
	s.n = n
	return l, n == -1
}

func (s *DomainScanner) PrevSubDomain() (sub string, end bool) {
	n := strings.LastIndexByte(s.d[:s.n], '.')
	s.n = n
	return s.d[n+1:], n == -1
}
