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

package fastforward

import "github.com/IrineSistiana/mosdns/v5/pkg/upstream"

type upstreamWrapper struct {
	upstream.Upstream
	cfg UpstreamConfig
}

// name returns upstream tag if it was set in the config.
// Otherwise, it returns upstream address.
func (u *upstreamWrapper) name() string {
	if t := u.cfg.Tag; len(t) > 0 {
		return u.cfg.Tag
	}
	return u.cfg.Addr
}
