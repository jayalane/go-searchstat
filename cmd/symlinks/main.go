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
	"path/filepath"
	"strconv"
	"strings"
)

var ml lll.Lll
var theSkipDirs []string

var theConfig *config.Config
var defaultConfig = `#
cwd = .
debugLevel = network
profListen = localhost:8002
skipDirList = .snapshot|.git
numWorkers = 20,40
# comments
`

func skipDir(dir string) bool {
	for _, dn := range theSkipDirs {
		if dn == dir {
			return true
		}
	}
	return false
}

func parseNumWorkers(sNums []string, depth int) []int {
	gNums := make([]int, depth)
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
		gNums[i] = n
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
	theSkipDirs = parseSkipDirs((*theConfig)["skipDirList"].StrVal)

	// then the callback to check the dirs
	app.SetHandler(0, // dirs
		func(sp treewalk.StringPath) {

			fullPath := append(sp.Path[:], sp.Name)
			fn := strings.Join(fullPath[:], "/")
			fn = filepath.Clean(fn)
			des, err := os.ReadDir(fn)
			if err != nil {
				ml.La("Error on ReadDir", sp.Name, err)
				return
			}
			count.Incr("dir-handler-readdir-ok")
			for _, de := range des {
				ml.Ln("Got a dirEntry", de.Name())
				count.Incr("dir-handler-dirent-got")

				pathNew := append(sp.Path[:], sp.Name)
				spNew := treewalk.StringPath{Name: de.Name(), Path: pathNew[:]}

				if de.IsDir() {
					count.Incr("dir-handler-dirent-got-dir")
					if skipDir(de.Name()) {
						ml.Ls("Skipping", de.Name())
						count.Incr("dir-handler-dirent-skip")
						continue
					}
					newPath := append(spNew.Path, spNew.Name)
					deDn := strings.Join(newPath, "/") // direntry Dir Name
					fi, err := os.Lstat(deDn)
					if err != nil {
						ml.La("Stat error on", deDn, err)
						count.Incr("dir-handler-stat-error")
						return
					}
					if fi.Mode()&os.ModeSymlink == os.ModeSymlink { // the logic specific to this app
						lt, err := os.Readlink(deDn)
						if err != nil {
							ml.La("Readlink error on", deDn, err)
							count.Incr("dir-handler-readlink-error")
							return
						}
						count.Incr("dir-handler-symlink")
						fmt.Println(deDn, "==>", lt)
					}
					app.SendOn(0, de.Name(), sp)
					count.Incr("dir-handler-dirent-got-dir")
				} else {
					count.Incr("dir-handler-dirent-got-not-dir")
					app.SendOn(1, de.Name(), sp)
				}
			}
		})

	// then the callback to print the files
	app.SetHandler(1, // files
		func(sp treewalk.StringPath) {
			fullPath := append(sp.Path, sp.Name)
			fn := strings.Join(fullPath, "/")
			fi, err := os.Lstat(fn)
			if err != nil {
				ml.La("Stat error on", fn, err)
				count.Incr("file-handler-stat-error")
				return
			}
			if fi.Mode()&os.ModeSymlink == os.ModeSymlink { // the logic specific to this app
				lt, err := os.Readlink(fn)
				if err != nil {
					ml.La("Readlink error on", fn, err)
					count.Incr("file-handler-readlink-error")
					return
				}
				fmt.Println(fn, "==>", lt)
				count.Incr("file-handler-symlink")
			}
		})
	app.Start()
	app.Wait()
	count.LogCounters()
}
