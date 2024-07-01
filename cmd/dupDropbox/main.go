// -*- tab-width: 2 -*-

package main

import (
	"fmt"
	_ "net/http/pprof" //nolint:gosec
	"os"
	"strconv"
	"strings"

	count "github.com/jayalane/go-counter"
	globals "github.com/jayalane/go-globals"
	set "github.com/jayalane/go-persist-set"
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

func dirAFileHandler(sp treewalk.StringPath, dirAFiles *set.SetDb) {
	fullPath := append(sp.Path, sp.Name) //nolint:gocritic
	fn := strings.Join(fullPath, "/")

	fi, err := os.Lstat(fn)
	if err != nil {
		count.IncrSuffix("file-handler-stat-err", "handler")
		g.Ml.La("Error on stat", fn, err)

		return
	}

	if !fi.Mode().IsRegular() {
		count.IncrSuffix("file-handler-skip-not-regular", "handler")
		g.Ml.La("Skipping file is not regular", fn)

		return
	}

	f, err := os.Open(fn)
	if err != nil {
		count.IncrSuffix("file-handler-open-err", "handler")
		g.Ml.La("Error opening", fn, err)

		return
	}

	hash, err := hashReadCloser(f)
	if err != nil {
		count.IncrSuffix("file-handler-hash-read-err", "handler")
		g.Ml.La("Error reading for hash", fn, err)

		return
	}

	dirAFiles.Add(fn + ":" + hash)
	count.IncrSuffix("file-handler-ok", "handler")
}

func dirBFileHandler(sp treewalk.StringPath, dirAFiles *set.SetDb) {
	fullPath := append(sp.Path, sp.Name) //nolint:gocritic
	fn := strings.Join(fullPath, "/")

	fi, err := os.Lstat(fn)
	if err != nil {
		count.IncrSuffix("file-handler-stat-err", "handlerb")
		g.Ml.La("Error on stat", fn, err)

		return
	}

	if !fi.Mode().IsRegular() {
		count.IncrSuffix("file-handler-skip-not-regular", "handlerb")
		g.Ml.La("Skipping file is not regular", fn)

		return
	}

	f, err := os.Open(fn)
	if err != nil {
		count.IncrSuffix("file-handler-open-err", "handlerb")
		g.Ml.La("Error opening", fn, err)

		return
	}

	hash, err := hashReadCloser(f)
	if err != nil {
		count.IncrSuffix("file-handler-hash-read-err", "handlerb")
		g.Ml.La("Error reading for hash", fn, err)

		return
	}

	if dirAFiles.InSet(fn + ":" + hash) {
		count.IncrSuffix("file-handler-found", "handlerb")
		fmt.Println(fn)
	} else {
		count.IncrSuffix("file-handler-not-found", "handlerb")
	}
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
	theDirA := (*g.Cfg)["cwd1"].StrVal
	theDirB := (*g.Cfg)["cwd2"].StrVal
	depth := 2
	appA := treewalk.New(theDirA, depth)
	appB := treewalk.New(theDirB, depth)

	// then the worker numbers
	sNums := strings.Split((*g.Cfg)["numWorkers"].StrVal, ",")
	gNums := parseNumWorkers(sNums, depth)
	appA.SetNumWorkers(gNums)
	appB.SetNumWorkers(gNums)

	// then the directories to skip
	skipDirs := parseSkipDirs((*g.Cfg)["skipDirList"].StrVal)
	appA.SetSkipDirs(skipDirs)
	appB.SetSkipDirs(skipDirs)
	// log level
	appA.SetLogLevel((*g.Cfg)["debugLevel"].StrVal)
	appB.SetLogLevel((*g.Cfg)["debugLevel"].StrVal)

	// define the dedup map
	fileMap := set.New("diffingDirFiles")

	// Now populate the fileMap
	appA.SetHandler(
		1, // files
		func(sp treewalk.StringPath) {
			dirAFileHandler(sp, fileMap)
		},
	)

	appA.Start()
	appA.Wait()

	// Now check dir B
	appB.SetHandler(
		1, // files
		func(sp treewalk.StringPath) {
			dirBFileHandler(sp, fileMap)
		},
	)

	appB.Start()
	appB.Wait()
	count.LogCounters()
}
