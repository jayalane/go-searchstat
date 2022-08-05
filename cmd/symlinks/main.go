// -*- tab-width: 2 -*-

package main

import (
	"fmt"
	count "github.com/jayalane/go-counter"
	lll "github.com/jayalane/go-lll"
	config "github.com/jayalane/go-tinyconfig"
	treewalk "github.com/jayalane/go-treewalk"
	"github.com/pkg/profile"
	"io/fs"
	"net/http"
	_ "net/http/pprof"
	"os"
	"strconv"
	"strings"
)

var ml lll.Lll

var theConfig *config.Config
var defaultConfig = `#
cwd = .
debugLevel = network
profListen = localhost:8002
skipDirList = .snapshot|.git
numWorkers = 20,40
# comments
`

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

func main() {

	suffix := "main"

	// CPU profile
	defer profile.Start(profile.ProfilePath(".")).Stop()
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
	ml = lll.Init("SEARCH", (*theConfig)["debugLevel"].StrVal)

	// stats
	count.InitCounters()

	// first start directory
	theDir := (*theConfig)["cwd"].StrVal
	depth := 2
	app := treewalk.New(theDir, depth)

	// then the worker numbers
	sNums := strings.Split((*theConfig)["numWorkers"].StrVal, ",")
	gNums := parseNumWorkers(sNums, depth)
	app.SetNumWorkers(gNums)

	// then the directories to skip
	app.SetSkipDirs(parseSkipDirs((*theConfig)["skipDirList"].StrVal))

	// the callback to print the files link data
	app.SetHandler(1, // files
		func(sp treewalk.StringPath) {
			isSymLink := false
			fullPath := append(sp.Path, sp.Name)
			fn := strings.Join(fullPath, "/")
			var fi fs.FileInfo
			var err error
			de, ok := sp.Value.(fs.DirEntry)
			if ok {
				fi, err = de.Info()
				count.Incr("Used de")
			} else {
				count.Incr("Used Lstat")
				fi, err = treewalk.Lstat(fn)
			}
			if err != nil {
				ml.La("Stat error on", fn, err)
				count.IncrSuffix("file-handler-stat-error", suffix)
				return
			}
			isSymLink = fi.Mode()&os.ModeSymlink == os.ModeSymlink
			if isSymLink { // the logic specific to this app
				lt, err := os.Readlink(fn)
				if err != nil {
					ml.La("Readlink error on", fn, err)
					count.IncrSuffix("file-handler-readlink-error", suffix)
					return
				}
				fmt.Println(fn, "==>", lt)
				count.IncrSuffix("file-handler-symlink", suffix)
			}
		})
	app.Start()
	app.Wait()
	count.LogCounters()
}
