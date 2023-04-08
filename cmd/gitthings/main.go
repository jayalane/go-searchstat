// -*- tab-width: 2 -*-

package main

import (
	"fmt"
	count "github.com/jayalane/go-counter"
	lll "github.com/jayalane/go-lll"
	nonblocking "github.com/jayalane/go-syscalls-timeout"
	config "github.com/jayalane/go-tinyconfig"
	treewalk "github.com/jayalane/go-treewalk"
	"github.com/pkg/profile"
	"net/http"
	_ "net/http/pprof"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var ml *lll.Lll

var gSkips []string

var gitRE *regexp.Regexp
var theConfig *config.Config
var defaultConfig = `#
cwd = .
debugLevel = network
profListen = localhost:8002
skipDirList = .snapshot|.git
numWorkers = 20,40
gitFileRE = (^git\ pull|\ git\ pull)
# comments
`

func parseSkipDirs(str string) []string {
	splits := strings.Split(str, "|")
	length := len(splits)
	res := make([]string, length)
	if length == 0 {
		return res
	}
	copy(res, splits)
	return res
}

func skipDir(d string) bool {
	for _, x := range gSkips {
		if x == d {
			return true
		}
	}
	return false
}

// parseNumWorkers turns a config line '30,40' into a slice
// of ints [30, 40]
func parseNumWorkers(sNums []string, depth int) []int64 {
	gNums := make([]int64, depth)
	if len(sNums) != depth {
		s := fmt.Sprintln("misconfigured numWorkers",
			(*theConfig)["numWorkers"])
		panic(s)
	}
	for i, k := range sNums {
		n, err := strconv.Atoi(k)
		if err != nil {
			s := fmt.Sprintln("misconfigured numWorkers",
				(*theConfig)["numWorkers"], k, err)
			panic(s)
		}
		gNums[i] = int64(n)
	}
	return gNums
}

func main() {
	// defer profile.Start(profile.ProfilePath(".")).Stop()
	// BlockProfile enables block (contention) profiling.
	// defer profile.Start(profile.BlockProfile).Stop()
	// MutexProfile enables mutex profiling.
	defer profile.Start(profile.MutexProfile).Stop()

	// config
	if len(os.Args) > 1 && os.Args[1] == "--dumpConfig" {
		fmt.Println(defaultConfig)
		return
	}
	// still config
	theConfig = nil
	t, err := config.ReadConfig("config.txt", defaultConfig)
	if err != nil {
		fmt.Println("Error opening config.txt", err.Error())
		if theConfig == nil {
			os.Exit(11)
		}
	}
	theConfig = &t
	fmt.Println("Config", (*theConfig)) // lll isn't up yet

	// start the profiler
	go func() {
		if len((*theConfig)["profListen"].StrVal) > 0 {
			fmt.Println(http.ListenAndServe((*theConfig)["profListen"].StrVal, nil))
		}
	}()

	// low level logging (first so everything rotates)
	ml = lll.Init("GITTHINGS", (*theConfig)["debugLevel"].StrVal)
	suffix := "main"
	// also have to do treewalk app logger

	// stats
	count.InitCounters()

	// regexp for git files
	gitRE = regexp.MustCompile((*theConfig)["gitFileRE"].StrVal)

	// first start directory
	theDir := (*theConfig)["cwd"].StrVal
	depth := 2
	app := treewalk.New(theDir, depth)

	// then the worker numbers
	sNums := strings.Split((*theConfig)["numWorkers"].StrVal, ",")
	gNums := parseNumWorkers(sNums, depth)
	app.SetNumWorkers(gNums)

	// then the directories to skip
	gSkips = parseSkipDirs((*theConfig)["skipDirList"].StrVal)

	// log level
	app.SetLogLevel((*theConfig)["debugLevel"].StrVal)

	// defaultDirHandle is a default handler in the case this app is doing
	// find type search on a filesystem
	app.SetHandler(0, // override dir to print out .git dirs
		func(sp treewalk.StringPath) {
			fullPath := append(sp.Path[:], sp.Name)
			fn := strings.Join(fullPath[:], "/")
			fn = filepath.Clean(fn)
			des, err := nonblocking.ReadDir(fn)
			if err != nil {
				ml.La("Error on ReadDir", sp.Name, err)
				return
			}
			count.MarkDistributionSuffix("dir-handler-readdir-len", float64(len(des)),
				suffix)
			count.IncrSuffix("dir-handler-readdir-ok", suffix)
			for _, de := range des {
				count.IncrSuffix("dir-handler-dirent-got", suffix)
				ml.Ln("Got a dirEntry", de.Name())
				spNew := treewalk.StringPath{Name: de.Name(), Path: fullPath, Value: de}
				if de.IsDir() {
					if de.Name() == ".git" {
						fmt.Println(fn + "/" + de.Name())
						continue
					}
					if skipDir(de.Name()) {
						ml.Ls("Skipping", de.Name())
						count.IncrSuffix("dir-handler-dirent-skip", suffix)
						continue
					}
					count.IncrSuffix("dir-handler-dirent-got-dir", suffix)
					go app.SendOn(0, de.Name(), spNew)
				} else {
					app.SendOn(1, de.Name(), spNew)
					count.IncrSuffix("dir-handler-dirent-got-not-dir", suffix)
				}
			}
		})
	// then the callback to dedup the files
	app.SetHandler(1, // files
		func(sp treewalk.StringPath) {
			fullPath := append(sp.Path, sp.Name)
			fn := strings.Join(fullPath, "/")
			fi, err := os.Lstat(fn)
			if err != nil {
				count.IncrSuffix("file-handler-stat-err", suffix)
				ml.La("Error on stat", fn, err)
				return
			}
			if !fi.Mode().IsRegular() {
				count.IncrSuffix("file-handler-skip-not-regular", suffix)
				ml.La("Skipping file is not regular", fn)
				return
			}
			f, err := os.Open(fn)
			if err != nil {
				count.IncrSuffix("file-handler-open-err", suffix)
				ml.La("Error opening", fn, err)
				return
			}
			gitFile, err := findString(fn, f, "git ") // closes f
			if err != nil {
				count.IncrSuffix("file-handler-grep-read-err", suffix)
				ml.La("Error reading for grep", fn, err)
				return
			}
			if gitFile {
				count.IncrSuffix("file-handler-gitfile", suffix)
				fmt.Println(fn)
			} else {
				count.IncrSuffix("file-handler-not-gitfile", suffix)
			}
			count.IncrSuffix("file-handler-ok", suffix)
		})
	app.Start()
	app.Wait()
	count.LogCounters()
}
