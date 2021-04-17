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

package http_handler

import "time"

type Option func(h *Handler)

// WithPath sets the server entry point url path.
// If empty, Handler will not check the request path.
func WithPath(s string) Option {
	return func(h *Handler) {
		h.path = s
	}
}

// WithClientSrcIPHeader sets the header that Handler can read client's
// source IP when server is behind a proxy.
// e.g. "X-Forwarded-For" (nginx).
func WithClientSrcIPHeader(s string) Option {
	return func(h *Handler) {
		h.clientSrcIPHeader = s
	}
}

// WithTimeout sets the query maximum executing time.
// Default is defaultTimeout.
func WithTimeout(d time.Duration) Option {
	return func(h *Handler) {
		h.timeout = d
	}
}
