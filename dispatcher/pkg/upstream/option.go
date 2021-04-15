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

package upstream

import (
	"crypto/x509"
	"go.uber.org/zap"
	"time"
)

type Option func(u *FastUpstream) error

func WithSocks5(s string) Option {
	return func(u *FastUpstream) error {
		u.socks5 = s
		return nil
	}
}

func WithDialAddr(s string) Option {
	return func(u *FastUpstream) error {
		u.altDialAddr = s
		return nil
	}
}

func WithReadTimeout(d time.Duration) Option {
	return func(u *FastUpstream) error {
		u.readTimeout = d
		return nil
	}
}

func WithIdleTimeout(d time.Duration) Option {
	return func(u *FastUpstream) error {
		u.idleTimeout = d
		return nil
	}
}

func WithMaxConns(n int) Option {
	return func(u *FastUpstream) error {
		u.maxConns = n
		return nil
	}
}

func WithRootCAs(cp *x509.CertPool) Option {
	return func(u *FastUpstream) error {
		u.rootCAs = cp
		return nil
	}
}

func WithInsecureSkipVerify(b bool) Option {
	return func(u *FastUpstream) error {
		u.insecureSkipVerify = b
		return nil
	}
}

func WithLogger(l *zap.Logger) Option {
	return func(u *FastUpstream) error {
		u.logger = l
		return nil
	}
}
