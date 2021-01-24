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

package coremain

import (
	"github.com/IrineSistiana/mosdns/dispatcher/handler"
	"gopkg.in/yaml.v3"
	"io/ioutil"
	"os"
)

// Config is config
type Config struct {
	Log struct {
		Level string `yaml:"level"`
		File  string `yaml:"file"`
	} `yaml:"log"`
	Library []string          `yaml:"library"`
	Plugin  []*handler.Config `yaml:"plugin"`
	Include []string          `yaml:"include"`
}

// parseConfig loads a yaml config from path f.
func parseConfig(f string) (*Config, error) {
	b, err := ioutil.ReadFile(f)
	if err != nil {
		return nil, err
	}

	c := new(Config)
	if err := yaml.Unmarshal(b, c); err != nil {
		return nil, err
	}

	return c, nil
}

// GenConfig generates a config template to path p.
func GenConfig(p string) error {
	c := new(Config)
	c.Log.Level = "info"

	argsPlaceholder := map[string]interface{}{"arg1": "value1", "arg2": "value2"}
	c.Plugin = append(c.Plugin, &handler.Config{
		Tag:  "server",
		Type: "server",
		Args: argsPlaceholder,
	})

	c.Plugin = append(c.Plugin, &handler.Config{
		Tag:  "forward",
		Type: "forward",
		Args: argsPlaceholder,
	})
	return c.Save(p)
}

func (c *Config) Save(p string) error {
	f, err := os.Create(p)
	if err != nil {
		return err
	}
	defer f.Close()

	encoder := yaml.NewEncoder(f)
	encoder.SetIndent(2)
	defer encoder.Close()
	err = encoder.Encode(c)
	if err != nil {
		return err
	}

	return err
}
