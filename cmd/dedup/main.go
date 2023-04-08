// -*- tab-width: 2 -*-

package main

import (
	"fmt"
	count "github.com/jayalane/go-counter"
	dedup "github.com/jayalane/go-dedup-map"
	lll "github.com/jayalane/go-lll"
	config "github.com/jayalane/go-tinyconfig"
	treewalk "github.com/jayalane/go-treewalk"
	"github.com/pkg/profile"
	"net/http"
	_ "net/http/pprof"
	"os"
	"strconv"
	"strings"
)

var ml *lll.Lll

var theConfig *config.Config
var defaultConfig = `#
cwd = .
debugLevel = network
profListen = localhost:8002
skipDirList = .snapshot|.git
numWorkers = 20,40
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
	ml = lll.Init("DEDUP", (*theConfig)["debugLevel"].StrVal)
	// also have to do treewalk app logger

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
	skipDirs := parseSkipDirs((*theConfig)["skipDirList"].StrVal)
	app.SetSkipDirs(skipDirs)
	// log level
	app.SetLogLevel((*theConfig)["debugLevel"].StrVal)

	// define the dedup map
	dedupMap := dedup.New("dedupingFiles")

	// then the callback to dedup the files
	app.SetHandler(1, // files
		func(sp treewalk.StringPath) {
			fullPath := append(sp.Path, sp.Name)
			fn := strings.Join(fullPath, "/")
			fi, err := os.Lstat(fn)
			if err != nil {
				count.Incr("file-handler-stat-err")
				ml.La("Error on stat", fn, err)
				return
			}
			if !fi.Mode().IsRegular() {
				count.Incr("file-handler-skip-not-regular")
				ml.La("Skipping file is not regular", fn)
				return
			}
			f, err := os.Open(fn)
			if err != nil {
				count.Incr("file-handler-open-err")
				ml.La("Error opening", fn, err)
				return
			}
			hash, err := hashReadCloser(f)
			if err != nil {
				count.Incr("file-handler-hash-read-err")
				ml.La("Error reading for hash", fn, err)
				return
			}
			dedupMap.Set(hash, fn)
			count.Incr("file-handler-ok")
		})
	app.Start()
	app.Wait()
	dups := dedupMap.GetDups()
	for k, v := range dups {
		fmt.Println(k, ",", v)
	}
	count.LogCounters()
}
