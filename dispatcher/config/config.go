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
	"gopkg.in/yaml.v3"
	"io/ioutil"
	"os"
)

// Config is config
type Config struct {
	Plugin []*handler.Config `yaml:"plugin"`
}

// LoadConfig loads a yaml config from path p.
func LoadConfig(p string) (*Config, error) {
	c := new(Config)
	b, err := ioutil.ReadFile(p)
	if err != nil {
		return nil, err
	}

	if err := yaml.Unmarshal(b, c); err != nil {
		return nil, err
	}

	return c, nil
}

// GenConfig generates a config template to path p.
func GenConfig(p string) error {
	c, err := GetTemplateConfig()
	if err != nil {
		return err
	}

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

func (c *Config) AddPlugin(tag, typ string, args interface{}) error {
	out, err := objToGeneral(args)
	if err != nil {
		return err
	}

	c.Plugin = append(c.Plugin, &handler.Config{
		Tag:  tag,
		Type: typ,
		Args: out,
	})
	return nil
}

func objToGeneral(in interface{}) (out map[string]interface{}, err error) {
	b, err := yaml.Marshal(in)
	if err != nil {
		return nil, err
	}

	out = make(map[string]interface{})
	err = yaml.Unmarshal(b, out)
	if err != nil {
		return nil, err
	}
	return out, nil
}
