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

package coremain

import (
	"github.com/IrineSistiana/mosdns/v4/mlog"
	"github.com/IrineSistiana/mosdns/v4/pkg/data_provider"
	"github.com/IrineSistiana/mosdns/v4/pkg/utils"
)

type Config struct {
	Log           mlog.LogConfig                     `yaml:"log"`
	Include       []string                           `yaml:"include"`
	DataProviders []data_provider.DataProviderConfig `yaml:"data_providers"`
	Plugins       []PluginConfig                     `yaml:"plugins"`
	Servers       []ServerConfig                     `yaml:"servers"`
	API           APIConfig                          `yaml:"api"`

	// Experimental
	Security SecurityConfig `yaml:"security"`
}

// PluginConfig represents a plugin config
type PluginConfig struct {
	// Tag, required
	Tag string `yaml:"tag"`

	// Type, required
	Type string `yaml:"type"`

	// Args, might be required by some plugins.
	// The type of Args is depended on RegNewPluginFunc.
	// If it's a map[string]interface{}, it will be converted by mapstruct.
	Args interface{} `yaml:"args"`
}

type ServerConfig struct {
	Exec      string                  `yaml:"exec"`
	Timeout   uint                    `yaml:"timeout"` // (sec) query timeout.
	Listeners []*ServerListenerConfig `yaml:"listeners"`
}

type ServerListenerConfig struct {
	// Protocol: server protocol, can be:
	// "", "udp" -> udp
	// "tcp" -> tcp
	// "dot", "tls" -> dns over tls
	// "doh", "https" -> dns over https (rfc 8844)
	// "http" -> dns over https (rfc 8844) but without tls
	Protocol string `yaml:"protocol"`

	// Addr: server "host:port" addr.
	// Addr cannot be empty.
	Addr string `yaml:"addr"`

	Cert                string `yaml:"cert"`                    // certificate path, used by dot, doh
	Key                 string `yaml:"key"`                     // certificate key path, used by dot, doh
	URLPath             string `yaml:"url_path"`                // used by doh, http. If it's empty, any path will be handled.
	GetUserIPFromHeader string `yaml:"get_user_ip_from_header"` // used by doh, http.

	IdleTimeout uint `yaml:"idle_timeout"` // (sec) used by tcp, dot, doh as connection idle timeout.
}

type APIConfig struct {
	HTTP string `yaml:"http"`
}

type SecurityConfig struct {
	BadIPObserver BadIPObserverConfig `yaml:"bad_ip_observer"`
}

// BadIPObserverConfig is a copy of ip_observer.BadIPObserverOpts.
type BadIPObserverConfig struct {
	OnUpdateCallBack string `yaml:"on_update_callback"`

	Threshold int `yaml:"threshold"` // Default is 500.
	Interval  int `yaml:"interval"`  // (sec) Default is 10.
	TTL       int `yaml:"ttl"`       // (sec) Default is 600 (10min).
	// IP masks to aggregate an IP range.
	IPv4Mask int `yaml:"ipv4_mask"` // Default is 32.
	IPv6Mask int `yaml:"ipv6_mask"` // Default is 48.
}

func (c *BadIPObserverConfig) Init() {
	utils.SetDefaultNum(&c.Threshold, 500)
	utils.SetDefaultNum(&c.Interval, 10)
	utils.SetDefaultNum(&c.TTL, 600)
	utils.SetDefaultNum(&c.IPv4Mask, 32)
	utils.SetDefaultNum(&c.IPv6Mask, 48)
}
