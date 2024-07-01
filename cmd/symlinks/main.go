// -*- tab-width: 2 -*-

package main

import (
	"fmt"
	"io/fs"
	_ "net/http/pprof" //nolint:gosec
	"os"
	"strconv"
	"strings"

	count "github.com/jayalane/go-counter"
	globals "github.com/jayalane/go-globals"
	timeout "github.com/jayalane/go-syscalls-timeout"
	treewalk "github.com/jayalane/go-treewalk"
)

const (
	errExit = 11
	suffix  = "main"
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

func symlinkFileHandler(sp treewalk.StringPath) {
	var fi fs.FileInfo

	var err error

	isSymLink := false

	fullPath := append(sp.Path, sp.Name) //nolint:gocritic

	fn := strings.Join(fullPath, "/")

	fi, ok := sp.Value.(fs.FileInfo)
	if ok {
		count.Incr("Used interface")
	} else {
		count.Incr("Used Lstat")

		fi, err = timeout.Lstat(fn)
	}

	if err != nil {
		g.Ml.La("Stat error on", fn, err)
		count.IncrSuffix("file-handler-stat-error", suffix)

		return
	}

	isSymLink = fi.Mode()&os.ModeSymlink == os.ModeSymlink
	if isSymLink { // the logic specific to this app
		lt, err := os.Readlink(fn)
		if err != nil {
			g.Ml.La("Readlink error on", fn, err)
			count.IncrSuffix("file-handler-readlink-error", suffix)

			return
		}

		fmt.Println(fn, "==>", lt)
		count.IncrSuffix("file-handler-symlink", suffix)
	}
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
	app.SetSkipDirs(parseSkipDirs((*g.Cfg)["skipDirList"].StrVal))

	// the callback to print the files link data
	app.SetHandler(
		1, // files
		symlinkFileHandler,
	)
	app.Start()
	app.Wait()
	count.LogCounters()
}
