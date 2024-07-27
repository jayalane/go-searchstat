// -*- tab-width: 2 -*-

package main

import (
	"errors"
	"fmt"
	"io"
	_ "net/http/pprof" //nolint:gosec
	"os"
	"path/filepath"
	"strconv"
	"strings"

	count "github.com/jayalane/go-counter"
	globals "github.com/jayalane/go-globals"
	treewalk "github.com/jayalane/go-treewalk"
)

const (
	errExit       = 11
	suffix        = "main"
	readdirBuffer = 50
)

var g globals.Global

var theApp treewalk.Treewalk

var defaultConfig = `#
cwd = .
debugLevel = network
profListen = localhost:8002
skipDirList = .snapshot|.git
numWorkers = 20,40
logStdout = true
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

func rmEmptyDir(sp treewalk.StringPath) { //nolint:cyclop
	fullPath := append(sp.Path, sp.Name) //nolint:gocritic
	fn := strings.Join(fullPath, "/")
	fn = filepath.Clean(fn)

	g.Ml.La("CB for dir", fn)

	dir, err := os.Open(fn)
	if err != nil {
		g.Ml.La("Error on OpenDir", sp.Name, err)
		count.IncrSuffix("dir-handler-readdir-open-err", suffix)

		return
	}

	dirEntryLen := 0

	rmDir := true
	done := false

	for {
		// this loop is inessential but needed as we only
		// get 50  dirents at a time
		des, err := dir.Readdir(readdirBuffer) // tunable ?
		if err != nil && !errors.Is(err, io.EOF) {
			g.Ml.La("Error on OpenDir", sp.Name, err)
			count.IncrSuffix("dir-handler-readdir-open-err", suffix)

			return
		}

		if err != nil && errors.Is(err, io.EOF) {
			// might be second loop or might be empty dir - so can't return
			done = true
		} else {

			dirEntryLen += len(des)
			g.Ml.Ln(fn, "entries:", len(des))

			for _, de := range des {
				rmDir = false
				spNew := treewalk.StringPath{Name: de.Name(), Path: fullPath, Value: de}

				count.IncrSuffix("dir-handler-dirent-got", suffix)

				if de.IsDir() {
					g.Ml.Ln("Got a dirEntry dir", strings.Join(fullPath, "/")+"/"+de.Name())

					go theApp.SendOn(0, de.Name(), spNew) // the go is needed to avoid a deadlock
				} else {
					count.IncrSuffix("dir-handler-dirent-got-not-dir", suffix)
				}
			}
		}

		count.MarkDistributionSuffix("dir-handler-readdir-len", float64(dirEntryLen), suffix)
		count.IncrSuffix("dir-handler-readdir-ok", suffix)

		if done {
			if rmDir {
				g.Ml.Ls("Would remove empty dir", sp.Name, fn)

				err := os.Remove(fn)
				if err != nil {
					g.Ml.La(fn, "RM got error", err)
					count.IncrSuffix("dir-handler-remove-err", suffix)
				} else {
					count.IncrSuffix("dir-handler-remove-ok", suffix)
				}
			}

			return
		}
	}
}

func main() {
	g = globals.NewGlobal(defaultConfig, true)

	// first start directory
	theDir := (*g.Cfg)["cwd"].StrVal
	depth := 2
	theApp = treewalk.New(theDir, depth)

	// then the worker numbers
	sNums := strings.Split((*g.Cfg)["numWorkers"].StrVal, ",")
	gNums := parseNumWorkers(sNums, depth)
	theApp.SetNumWorkers(gNums)

	theApp.SetHandler(
		0, // dirs
		rmEmptyDir,
	)
	theApp.Start()
	theApp.Wait()
	count.LogCounters()
}
