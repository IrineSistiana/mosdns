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

package main

import (
	"flag"
	"fmt"
	"github.com/IrineSistiana/mosdns/dispatcher/coremain"
	"github.com/IrineSistiana/mosdns/dispatcher/mlog"
	"github.com/IrineSistiana/mosdns/tools"
	"os"
	"path/filepath"
	"runtime"

	//DEBUG ONLY
	"net/http"
	_ "net/http/pprof"
)

var (
	version = "dev/unknown"

	configPath  = flag.String("c", "config.yaml", "[path] load config from file")
	genConfigTo = flag.String("gen", "", "[path] generate a config template here")

	dir                 = flag.String("dir", "", "[path] change working directory to here")
	dirFollowExecutable = flag.Bool("dir2exe", false, "change working directory to the executable that started the current process")

	showVersion = flag.Bool("v", false, "show version info")

	probeServerTimeout = flag.String("probe-server-timeout", "", "[protocol://ip:port] probe server's idle timeout, protocol can be tcp or dot")

	benchIPMatcherFile     = flag.String("bench-ip-matcher", "", "[path] benchmark ip search using this file")
	benchDomainMatcherFile = flag.String("bench-domain-matcher", "", "[path] benchmark domain search using this file")

	convV2IPDat     = flag.String("conv-v2ray-ip-dat", "", "[path] convert v2ray ip data file to text")
	convV2DomainDat = flag.String("conv-v2ray-domain-dat", "", "[path] convert v2ray domain data file to text")

	//DEBUG ONLY
	cpu       = flag.Int("cpu", runtime.NumCPU(), "the maximum number of CPUs that can be executing simultaneously")
	pprofAddr = flag.String("pprof", "", "[ip:port] DEBUG ONLY, hook http/pprof at this address")
)

func main() {
	flag.Parse()

	// DEBUG ONLY
	runtime.GOMAXPROCS(*cpu)
	if len(*pprofAddr) != 0 {
		go func() {
			mlog.S().Infof("pprof backend is starting at: %v", *pprofAddr)
			if err := http.ListenAndServe(*pprofAddr, nil); err != nil {
				mlog.S().Fatalf("pprof backend is exited: %v", err)
			}
		}()
	}

	// helper function

	// show version
	if *showVersion {
		fmt.Printf("%s\n", version)
		os.Exit(0)
	}

	// idle timeout test
	if len(*probeServerTimeout) != 0 {
		err := tools.ProbServerTimeout(*probeServerTimeout)
		if err != nil {
			mlog.S().Error(err)
		}
		os.Exit(0)
	}

	// bench
	if len(*benchIPMatcherFile) != 0 {
		err := tools.BenchIPMatcher(*benchIPMatcherFile)
		if err != nil {
			mlog.S().Errorf("bench ip list failed, %v", err)
		}
		os.Exit(0)
	}
	if len(*benchDomainMatcherFile) != 0 {
		err := tools.BenchDomainMatcher(*benchDomainMatcherFile)
		if err != nil {
			mlog.S().Errorf("bench domain list failed, %v", err)
		}
		os.Exit(0)
	}

	// convert
	if len(*convV2IPDat) != 0 {
		err := tools.ConvertIPDat(*convV2IPDat)
		if err != nil {
			mlog.S().Error(err)
		}
		os.Exit(0)
	}
	if len(*convV2DomainDat) != 0 {
		err := tools.ConvertDomainDat(*convV2DomainDat)
		if err != nil {
			mlog.S().Error(err)
		}
		os.Exit(0)
	}

	// generate config
	if len(*genConfigTo) != 0 {
		err := coremain.GenConfig(*genConfigTo)
		if err != nil {
			mlog.S().Errorf("can not generate config template, %v", err)
		} else {
			mlog.S().Info("config template generated")
		}
		os.Exit(0)
	}

	// main program starts here

	// show summary
	mlog.S().Infof("mosdns ver: %s", version)
	mlog.S().Infof("arch: %s, os: %s, go: %s", runtime.GOARCH, runtime.GOOS, runtime.Version())

	// try to change working dir to os.Executable() or *dir
	var wd string
	if *dirFollowExecutable {
		ex, err := os.Executable()
		if err != nil {
			mlog.S().Fatalf("failed to get executable path: %v", err)
		}
		wd = filepath.Dir(ex)
	} else {
		if len(*dir) != 0 {
			wd = *dir
		}
	}
	if len(wd) != 0 {
		err := os.Chdir(wd)
		if err != nil {
			mlog.S().Fatalf("failed to change the current working directory: %v", err)
		}
		mlog.S().Infof("current working directory: %s", wd)
	}

	coremain.Run(*configPath)
}
