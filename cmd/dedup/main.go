// -*- tab-width: 2 -*-

package main

import (
	"fmt"
	_ "net/http/pprof" //nolint:gosec
	"os"
	"strconv"
	"strings"

	count "github.com/jayalane/go-counter"
	dedup "github.com/jayalane/go-dedup-map"
	globals "github.com/jayalane/go-globals"
	treewalk "github.com/jayalane/go-treewalk"
)

var g globals.Global

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

func dedupFileHandler(sp treewalk.StringPath, dedupMap *dedup.Dedup) {
	fullPath := append(sp.Path, sp.Name) //nolint:gocritic
	fn := strings.Join(fullPath, "/")

	fi, err := os.Lstat(fn)
	if err != nil {
		count.Incr("file-handler-stat-err")
		g.Ml.La("Error on stat", fn, err)

		return
	}

	if !fi.Mode().IsRegular() {
		count.Incr("file-handler-skip-not-regular")
		g.Ml.La("Skipping file is not regular", fn)

		return
	}

	f, err := os.Open(fn)
	if err != nil {
		count.Incr("file-handler-open-err")
		g.Ml.La("Error opening", fn, err)

		return
	}

	hash, err := hashReadCloser(f)
	if err != nil {
		count.Incr("file-handler-hash-read-err")
		g.Ml.La("Error reading for hash", fn, err)

		return
	}

	dedupMap.Set(hash, fn)
	count.Incr("file-handler-ok")
}

// parseNumWorkers turns a config line '30,40' into a slice
// of ints [30, 40].
func parseNumWorkers(sNums []string, depth int) []int64 {
	if len(sNums) != depth {
		s := fmt.Sprintln("misconfigured numWorkers",
			(*g.Cfg)["numWorkers"])
		panic(s)
	}

	gNums := make([]int64, depth)

	for i, k := range sNums {
		n, err := strconv.Atoi(k)
		if err != nil {
			s := fmt.Sprintln("misconfigured numWorkers",
				(*g.Cfg)["numWorkers"], k, err)
			panic(s)
		}

		gNums[i] = int64(n)
	}

	return gNums
}

func main() {
	g = globals.NewGlobal(defaultConfig, true)

	// first start directory
	theDir := (*g.Cfg)["cwd"].StrVal
	depth := 2
	app := treewalk.New(theDir, depth)

	// then the worker numbers
	sNums := strings.Split((*g.Cfg)["numWorkers"].StrVal, ",")
	gNums := parseNumWorkers(sNums, depth)
	app.SetNumWorkers(gNums)

	// then the directories to skip
	skipDirs := parseSkipDirs((*g.Cfg)["skipDirList"].StrVal)
	app.SetSkipDirs(skipDirs)
	// log level
	app.SetLogLevel((*g.Cfg)["debugLevel"].StrVal)

	// define the dedup map
	dedupMap := dedup.New("dedupingFiles")

	// then the callback to dedup the files
	app.SetHandler(
		1, // files
		func(sp treewalk.StringPath) {
			dedupFileHandler(sp, dedupMap)
		},
	)

	app.Start()
	app.Wait()

	dups := dedupMap.GetDups()
	for k, v := range dups {
		fmt.Println(k, ",", v)
	}

	count.LogCounters()
}
