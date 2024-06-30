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
	treewalk "github.com/jayalane/go-treewalk"
)

var g *globals.Global

var defaultConfig = `#
cwd = .
debugLevel = network
profListen = localhost:8002
skipDirList = .snapshot|.git
numWorkers = 20,40
# comments
`

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

func statFileHandler(sp treewalk.StringPath) {
	fullPath := append(sp.Path, sp.Name) //nolint:gocritic
	fn := strings.Join(fullPath, "/")

	fi, err := os.Lstat(fn)
	if err != nil {
		g.Ml.La("Stat error on", fn, err)

		return
	}

	if fi.Size() == 0 { // the logic specific to this app
		fmt.Println(fn, fi.ModTime())
	}
}

func main() {
	g := globals.NewGlobal(defaultConfig, true)

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
	app.SetSkipDirs(skipDirs) // skip e.g. .snapshot on NAS

	// then the callback to print the files
	app.SetHandler(
		1, // files
		statFileHandler,
	)
	app.Start()
	app.Wait()
	count.LogCounters()
}
