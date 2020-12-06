//     Copyright (C) 2020, IrineSistiana
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

package config

import (
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/dispatcher/utils"
)

func GetTemplateConfig() (*Config, error) {
	c := new(Config)
	c.Server.Bind = []string{
		"udp://127.0.0.1:53",
		"tcp://127.0.0.1:53",
		"udp://[::1]:53",
		"tcp://[::1]:53",
	}
	c.Server.MaxUDPSize = utils.IPv4UdpMaxPayload

	c.Plugin.Entry = []string{"", ""}
	c.Plugin.Plugin = append(c.Plugin.Plugin, &handler.Config{})

	return c, nil
}
