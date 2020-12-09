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

package dispatcher

import (
	"fmt"
	"github.com/IrineSistiana/mosdns/dispatcher/config"
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"github.com/IrineSistiana/mosdns/dispatcher/mlog"
	_ "github.com/IrineSistiana/mosdns/dispatcher/plugin"
)

// Init loads plugins from config
func Init(c *config.Config) error {
	for i, pluginConfig := range c.Plugin {
		if len(pluginConfig.Tag) == 0 {
			mlog.Entry().Warnf("plugin at index %d has a empty tag, ignore it", i)
			continue
		}
		if err := handler.InitAndRegPlugin(pluginConfig); err != nil {
			return fmt.Errorf("failed to register plugin %d-%s: %w", i, pluginConfig.Tag, err)
		}
		mlog.Entry().Debugf("plugin %s loaded", pluginConfig.Tag)
	}
	return nil
}
