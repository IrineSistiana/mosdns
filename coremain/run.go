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
	"github.com/mitchellh/mapstructure"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"os"
	"runtime"
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
	if sf.cpu > 0 {
		runtime.GOMAXPROCS(sf.cpu)
	}

	if len(sf.dir) > 0 {
		err := os.Chdir(sf.dir)
		if err != nil {
			mlog.L().Fatal("failed to change the current working directory", zap.Error(err))
		}
		mlog.L().Info("working directory changed", zap.String("path", sf.dir))
	}

	v := viper.New()
	if len(sf.c) > 0 {
		v.SetConfigFile(sf.c)
	} else {
		v.SetConfigName("config")
		v.AddConfigPath(".")
	}

	if err := v.ReadInConfig(); err != nil {
		mlog.L().Fatal("failed to read config file", zap.Error(err))
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
