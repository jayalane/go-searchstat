// -*- tab-width: 2 -*-

package main

import (
	"fmt"
	_ "net/http/pprof" //nolint:gosec
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	count "github.com/jayalane/go-counter"
	globals "github.com/jayalane/go-globals"
	nonblocking "github.com/jayalane/go-syscalls-timeout"
	treewalk "github.com/jayalane/go-treewalk"
)

const (
	logSuffix = "main"
	errExit   = 11
)

var (
	g             globals.Global
	gSkips        []string
	gitRE         *regexp.Regexp
	defaultConfig = `#
cwd = .
debugLevel = network
profListen = localhost:8002
skipDirList = .snapshot|.git
numWorkers = 20,40
gitFileRE = (^git\ pull|\ git\ pull)
# comments
`
)

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

func gitTreeHandler(sp treewalk.StringPath) {
	fullPath := append(sp.Path, sp.Name) //nolint:gocritic
	fn := strings.Join(fullPath, "/")

	fi, err := os.Lstat(fn)
	if err != nil {
		count.IncrSuffix("file-handler-stat-err", logSuffix)
		g.Ml.La("Error on stat", fn, err)

		return
	}

	if !fi.Mode().IsRegular() {
		count.IncrSuffix("file-handler-skip-not-regular", logSuffix)
		g.Ml.La("Skipping file is not regular", fn)

		return
	}

	f, err := os.Open(fn)
	if err != nil {
		count.IncrSuffix("file-handler-open-err", logSuffix)
		g.Ml.La("Error opening", fn, err)

		return
	}

	gitFile, err := findString(fn, f, gitRE) // closes f
	if err != nil {
		count.IncrSuffix("file-handler-grep-read-err", logSuffix)
		g.Ml.La("Error reading for grep", fn, err)

		return
	}

	if gitFile {
		count.IncrSuffix("file-handler-gitfile", logSuffix)
		fmt.Println(fn)
	} else {
		count.IncrSuffix("file-handler-not-gitfile", logSuffix)
	}

	count.IncrSuffix("file-handler-ok", logSuffix)
}

func printGitFilesHandler(sp treewalk.StringPath, app *treewalk.Treewalk) {
	fullPath := append(sp.Path[:], sp.Name) //nolint:gocritic
	fn := strings.Join(fullPath, "/")
	fn = filepath.Clean(fn)

	des, err := nonblocking.ReadDir(fn)
	if err != nil {
		g.Ml.La("Error on ReadDir", sp.Name, err)

		return
	}

	count.MarkDistributionSuffix("dir-handler-readdir-len", float64(len(des)),
		logSuffix)
	count.IncrSuffix("dir-handler-readdir-ok", logSuffix)

	for _, de := range des {
		count.IncrSuffix("dir-handler-dirent-got", logSuffix)
		g.Ml.Ln("Got a dirEntry", de.Name())
		spNew := treewalk.StringPath{Name: de.Name(), Path: fullPath, Value: de}

		if de.IsDir() {
			if de.Name() == ".git" {
				fmt.Println(fn + "/" + de.Name())

				continue
			}

			if skipDir(de.Name()) {
				g.Ml.Ls("Skipping", de.Name())
				count.IncrSuffix("dir-handler-dirent-skip", logSuffix)

				continue
			}

			count.IncrSuffix("dir-handler-dirent-got-dir", logSuffix)

			go app.SendOn(0, de.Name(), spNew)
		} else {
			app.SendOn(1, de.Name(), spNew)
			count.IncrSuffix("dir-handler-dirent-got-not-dir", logSuffix)
		}
	}
}

func main() {
	g = globals.NewGlobal(defaultConfig, true)

	// pre-compile REs
	gitRE = regexp.MustCompile((*g.Cfg)["gitFileRE"].StrVal)

	// first start directory
	theDir := (*g.Cfg)["cwd"].StrVal
	depth := 2
	app := treewalk.New(theDir, depth)

	// then the worker numbers
	sNums := strings.Split((*g.Cfg)["numWorkers"].StrVal, ",")
	gNums := parseNumWorkers(sNums, depth)
	app.SetNumWorkers(gNums)

	// then the directories to skip
	gSkips = parseSkipDirs((*g.Cfg)["skipDirList"].StrVal)

	// log level
	app.SetLogLevel((*g.Cfg)["debugLevel"].StrVal)

	// defaultDirHandle is a default handler in the case this app is doing
	// find type search on a filesystem
	app.SetHandler(
		0, // override dir to print out .git dirs
		func(sp treewalk.StringPath) {
			printGitFilesHandler(sp, &app)
		},
	)
	// then the callback to dedup the files
	app.SetHandler(
		1, // files
		gitTreeHandler,
	)
	app.Start()
	app.Wait()
	count.LogCounters()
}
