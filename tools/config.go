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

package tools

import (
	"github.com/IrineSistiana/mosdns/v4/mlog"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"strings"
)

func newConvCmd() *cobra.Command {
	var (
		in  string
		out string
	)

	c := &cobra.Command{
		Use:   "conv -i input_cfg.yaml -o output_cfg.json",
		Args:  cobra.NoArgs,
		Short: "Convert configuration file format. Supported extensions: " + strings.Join(viper.SupportedExts, ", "),
		Run: func(cmd *cobra.Command, args []string) {
			if err := convCfg(in, out); err != nil {
				mlog.S().Fatal(err)
			}
		},
	}
	c.PersistentFlags().StringVarP(&in, "in", "i", "", "input config")
	c.PersistentFlags().StringVarP(&out, "out", "o", "", "output config")
	c.MarkFlagRequired("in")
	c.MarkFlagRequired("out")
	c.MarkFlagFilename("in")
	c.MarkFlagFilename("out")
	return c
}

func newGenCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "gen config.yaml",
		Short: "Generate a template config. Supported extensions: " + strings.Join(viper.SupportedExts, ", "),
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if err := genCfg(args[0]); err != nil {
				mlog.S().Fatal(err)
			}
		},
	}
	return c
}

func convCfg(in, out string) error {
	v := viper.New()
	v.SetConfigFile(in)
	if err := v.ReadInConfig(); err != nil {
		return err
	}
	return v.SafeWriteConfigAs(out)
}

func genCfg(out string) error {
	cfg := `
log:
  level: info
  file: ""

plugins:
  - tag: forward_google
    type: fast_forward
    args:
      upstream:
        - addr: https://8.8.8.8/dns-query

servers:
  - exec: forward_google
    listeners:
      - protocol: udp
        addr: 127.0.0.1:5533
      - protocol: tcp
        addr: 127.0.0.1:5533
`
	v := viper.New()
	v.SetConfigType("yaml")
	if err := v.ReadConfig(strings.NewReader(cfg)); err != nil {
		return err
	}

	return v.SafeWriteConfigAs(out)
}
