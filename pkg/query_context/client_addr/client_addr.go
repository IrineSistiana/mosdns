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

package client_addr

import (
	"github.com/IrineSistiana/mosdns/v5/pkg/query_context"
	"net/netip"
)

type key struct{}

func SetClientAddr(qCtx *query_context.Context, addr netip.Addr) {
	qCtx.SetKey((*key)(nil), addr)
}

func GetClientAddr(qCtx *query_context.Context) (netip.Addr, bool) {
	v, ok := qCtx.GetValue((*key)(nil))
	if !ok {
		return netip.Addr{}, false
	}
	return v.(netip.Addr), true
}
