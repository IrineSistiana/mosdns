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

package upstream

import (
	"fmt"
	"net"
	"net/netip"
	"strconv"
)

type socketOpts struct {
	so_mark        int
	bind_to_device string
}

func parseDialAddr(urlHost, dialAddr string, defaultPort uint16) (string, uint16, error) {
	addr := urlHost
	if len(dialAddr) > 0 {
		addr = dialAddr
	}
	host, port, err := trySplitHostPort(addr)
	if err != nil {
		return "", 0, err
	}
	if port == 0 {
		port = defaultPort
	}
	return host, port, nil
}

func joinPort(host string, port uint16) string {
	return net.JoinHostPort(host, strconv.Itoa(int(port)))
}

func tryRemovePort(s string) string {
	host, _, err := net.SplitHostPort(s)
	if err != nil {
		return s
	}
	return host
}

// trySplitHostPort splits host and port.
// If s has no port, it returns s,0,nil
func trySplitHostPort(s string) (string, uint16, error) {
	var port uint16
	host, portS, err := net.SplitHostPort(s)
	if err == nil {
		n, err := strconv.ParseUint(portS, 10, 16)
		if err != nil {
			return "", 0, fmt.Errorf("invalid port, %w", err)
		}
		port = uint16(n)
		return host, port, nil
	}
	return s, 0, nil
}

func parseBootstrapAp(s string) (netip.AddrPort, error) {
	host, port, err := trySplitHostPort(s)
	if err != nil {
		return netip.AddrPort{}, err
	}
	if port == 0 {
		port = 53
	}
	addr, err := netip.ParseAddr(host)
	if err != nil {
		return netip.AddrPort{}, err
	}
	return netip.AddrPortFrom(addr, port), nil
}

func tryTrimIpv6Brackets(s string) string {
	if len(s) < 2 {
		return s
	}
	if s[0] == '[' && s[len(s)-1] == ']' {
		return s[1 : len(s)-2]
	}
	return s
}

func msgTruncated(b []byte) bool {
	return b[2]&(1<<1) != 0
}
