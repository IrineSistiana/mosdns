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
	"fmt"
	"github.com/IrineSistiana/mosdns/v4/mlog"
	"github.com/mitchellh/mapstructure"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"os"
	"path/filepath"
	"runtime"
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
}

func AddSubCmd(c *cobra.Command) {
	rootCmd.AddCommand(c)
}

func Run() error {
	return rootCmd.Execute()
}

type serverFlags struct {
	c   string
	dir string
	cpu int
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
	if err := v.Unmarshal(cfg, decoderOpt); err != nil {
		mlog.L().Fatal("failed to parse config file", zap.Error(err))
	}

	cfgPath := v.ConfigFileUsed()
	if err := mergeInclude(cfg, 0, []string{cfgPath}, []string{tryGetAbsPath(cfgPath)}); err != nil {
		mlog.L().Fatal("failed to load sub config file", zap.Error(err))
	}

	if err := RunMosdns(cfg); err != nil {
		mlog.L().Fatal("mosdns exited", zap.Error(err))
	}
}

func decoderOpt(cfg *mapstructure.DecoderConfig) {
	cfg.ErrorUnused = true
	cfg.TagName = "yaml"
	cfg.WeaklyTypedInput = true
}

func mergeInclude(cfg *Config, depth int, paths, absPaths []string) error {
	depth++
	if depth > 8 {
		return fmt.Errorf("maximun include depth reached, include path is %s", strings.Join(paths, " -> "))
	}
	for _, subCfgFile := range cfg.Include {
		subPaths := append(paths, subCfgFile)
		subCfgAbsPath := tryGetAbsPath(subCfgFile)
		subAbsPaths := append(absPaths, subCfgAbsPath)
		for _, includedAbsPath := range absPaths {
			if includedAbsPath == subCfgAbsPath {
				return fmt.Errorf("cycle include depth detected, include path is %s", strings.Join(subPaths, " -> "))
			}
		}

		mlog.L().Info("reading sub config", zap.String("file", subCfgFile))
		subV := viper.New()
		subV.SetConfigFile(subCfgFile)
		if err := subV.ReadInConfig(); err != nil {
			mlog.L().Fatal("failed to read sub config file", zap.String("file", subCfgFile), zap.Error(err))
		}
		subCfg := new(Config)
		if err := subV.Unmarshal(subCfg, decoderOpt); err != nil {
			mlog.L().Fatal("failed to parse sub config file", zap.String("file", subCfgFile), zap.Error(err))
		}
		if err := mergeInclude(subCfg, depth, subPaths, subAbsPaths); err != nil {
			return err
		}

		cfg.DataProviders = append(cfg.DataProviders, subCfg.DataProviders...)
		cfg.Plugins = append(cfg.Plugins, subCfg.Plugins...)
		if len(subCfg.Servers) > 0 {
			mlog.L().Warn("server config in sub config files will be ignored", zap.String("file", subCfgFile))
		}
	}
	return nil
}

func tryGetAbsPath(s string) string {
	p, err := filepath.Abs(s)
	if err != nil {
		return s
	}
	return p
}
