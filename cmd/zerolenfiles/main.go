// -*- tab-width: 2 -*-

package main

import (
	"fmt"
	count "github.com/jayalane/go-counter"
	lll "github.com/jayalane/go-lll"
	config "github.com/jayalane/go-tinyconfig"
	treewalk "github.com/jayalane/go-treewalk"
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
	// now the treewalk config items

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
	app.SetSkipDirs(skipDirs) // skip e.g. .snapshot on NAS

	// then the callback to print the files
	app.SetHandler(1, // files
		func(sp treewalk.StringPath) {
			fullPath := append(sp.Path, sp.Name)
			fn := strings.Join(fullPath, "/")
			fi, err := os.Lstat(fn)
			if err != nil {
				ml.La("Stat error on", fn, err)
				return
			}
			if fi.Size() == 0 { // the logic specific to this app
				fmt.Println(fn, fi.ModTime())
			}
		})
	app.Start()
	app.Wait()
	count.LogCounters()
}
