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
	"bytes"
	"github.com/IrineSistiana/mosdns/v4/coremain/tools"
	"github.com/IrineSistiana/mosdns/v4/mlog"
	"github.com/mitchellh/mapstructure"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"os"
	"path/filepath"
	"strings"
)

var rootCmd = &cobra.Command{
	Use: "mosdns",
}

func init() {
	startCmd := &cobra.Command{
		Use:   "start",
		Short: "Start mosdns main program.",
		Run:   StartServer,
	}
	rootCmd.AddCommand(startCmd)
	fs := startCmd.PersistentFlags()
	fs.StringVarP(&sf.c, "config", "c", "", "config file")
	fs.StringVarP(&sf.dir, "dir", "d", "", "working dir")
	fs.IntVar(&sf.cpu, "cpu", 0, "set runtime.GOMAXPROCS")
	fs.StringVar(&sf.pprofAddr, "pprof", "", "start pprof server at this address")

	genCmd := &cobra.Command{
		Use:   "gen-config",
		Short: "Generate a template config.",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if err := GenConfig(args[0]); err != nil {
				mlog.S().Fatal(err)
			}
		},
	}
	rootCmd.AddCommand(genCmd)

	probeCmd := &cobra.Command{
		Use:   "probe",
		Short: "Run server tests. See sub commands for more details.",
	}
	probeCmd.AddCommand(tools.ConnReuse, tools.IdleTimeoutCmd, tools.Pipeline)
	rootCmd.AddCommand(probeCmd)

	v2datCmd := &cobra.Command{
		Use:   "v2dat",
		Short: "Tools that can unpack v2ray data file.",
	}
	v2datCmd.AddCommand(tools.UnpackDomain, tools.UnpackIP)
	rootCmd.AddCommand(v2datCmd)
}

func AddSubCmd(c *cobra.Command) {
	rootCmd.AddCommand(c)
}

func Run() error {
	return rootCmd.Execute()
}

type serverFlags struct {
	c         string
	dir       string
	cpu       int
	pprofAddr string
}

var sf = serverFlags{}

func StartServer(cmd *cobra.Command, args []string) {
	if len(sf.dir) > 0 {
		err := os.Chdir(sf.dir)
		if err != nil {
			mlog.L().Fatal("failed to change the current working directory", zap.Error(err))
		}
		mlog.L().Info("working directory changed", zap.String("path", sf.dir))
	}

	v := viper.New()
	v.SetEnvPrefix("mosdns")
	v.AutomaticEnv()
	v.SetConfigType("yaml")
	if len(sf.c) > 0 {
		b, err := os.ReadFile(sf.c)
		if err != nil {
			mlog.L().Fatal("failed to open config file", zap.Error(err))
		}

		if ext := filepath.Ext(sf.c); len(ext) > 0 {
			v.SetConfigType(strings.TrimPrefix(ext, "."))
		}

		if err := v.ReadConfig(bytes.NewReader(b)); err != nil {
			mlog.L().Fatal("failed to read config file", zap.Error(err))
		}
	} else {
		v.SetConfigName("config")
		v.AddConfigPath(".")
		if err := v.ReadInConfig(); err != nil {
			mlog.L().Fatal("failed to read config file", zap.Error(err))
		}
	}

	cfg := new(Config)
	if err := v.Unmarshal(cfg, func(cfg *mapstructure.DecoderConfig) {
		cfg.ErrorUnused = true
		cfg.TagName = "yaml"
		cfg.WeaklyTypedInput = true
	}); err != nil {
		mlog.L().Fatal("failed to parse config file", zap.Error(err))
	}

	if err := RunMosdns(cfg); err != nil {
		mlog.L().Fatal("mosdns exited", zap.Error(err))
	}
}
