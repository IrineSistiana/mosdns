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

package bootstrap

import (
	"context"
	"net"
	"strings"
)

// NewPlainBootstrap returns a customized *net.Resolver which Dial func is modified to dial s.
// s SHOULD be a literal IP address and the port SHOULD also be literal.
// Port can be omitted. In this case, the default port is :53.
// e.g. NewPlainBootstrap("8.8.8.8"), NewPlainBootstrap("127.0.0.1:5353")
// If s is empty, NewPlainBootstrap returns nil. (A nil *net.Resolver is valid in net.Dialer.)
// Note that not all platform support a customized *net.Resolver. It also depends on the
// version of go runtime.
// See the package docs from the net package for more info.
func NewPlainBootstrap(s string) *net.Resolver {
	if len(s) == 0 {
		return nil
	}
	// Add port.
	_, _, err := net.SplitHostPort(s)
	if err != nil { // no port, add it.
		s = net.JoinHostPort(strings.Trim(s, "[]"), "53")
	}

	return &net.Resolver{
		PreferGo:     true,
		StrictErrors: false,
		Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
			d := new(net.Dialer)
			return d.DialContext(ctx, network, s)
		},
	}
}
