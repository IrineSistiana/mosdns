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

package netlist

import (
	"bufio"
	"fmt"
	"github.com/IrineSistiana/mosdns/v5/pkg/utils"
	"io"
	"net/netip"
	"strings"
)

// LoadFromReader loads IP list from a reader.
// It might modify the List and causes List unsorted.
func LoadFromReader(l *List, reader io.Reader) error {
	scanner := bufio.NewScanner(reader)

	// count how many lines we have read.
	lineCounter := 0
	for scanner.Scan() {
		lineCounter++
		s := scanner.Text()
		s = strings.TrimSpace(s)
		s = utils.RemoveComment(s, "#")
		s = utils.RemoveComment(s, " ")
		if len(s) == 0 {
			continue
		}
		err := LoadFromText(l, s)
		if err != nil {
			return fmt.Errorf("invalid data at line #%d: %w", lineCounter, err)
		}
	}
	return scanner.Err()
}

// LoadFromText loads an IP from s.
// It might modify the List and causes List unsorted.
func LoadFromText(l *List, s string) error {
	if strings.ContainsRune(s, '/') {
		ipNet, err := netip.ParsePrefix(s)
		if err != nil {
			return err
		}
		l.Append(ipNet)
		return nil
	}

	addr, err := netip.ParseAddr(s)
	if err != nil {
		return err
	}
	bits := 32
	if addr.Is6() {
		bits = 128
	}
	l.Append(netip.PrefixFrom(addr, bits))
	return nil
}
