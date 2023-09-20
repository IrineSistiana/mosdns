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

package server_utils

import (
	"fmt"

	"github.com/IrineSistiana/mosdns/v5/coremain"
	"github.com/IrineSistiana/mosdns/v5/pkg/server"
	"github.com/IrineSistiana/mosdns/v5/pkg/server_handler"
	"github.com/IrineSistiana/mosdns/v5/plugin/executable/sequence"
)

func NewHandler(bp *coremain.BP, entry string) (server.Handler, error) {
	p := bp.M().GetPlugin(entry)
	exec := sequence.ToExecutable(p)
	if exec == nil {
		return nil, fmt.Errorf("cannot find executable entry by tag %s", entry)
	}

	handlerOpts := server_handler.EntryHandlerOpts{
		Logger: bp.L(),
		Entry:  exec,
	}
	return server_handler.NewEntryHandler(handlerOpts), nil
}
