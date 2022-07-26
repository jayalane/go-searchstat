// -*- tab-width: 2 -*-

package main

import (
	"fmt"
	lll "github.com/jayalane/go-lll"
	config "github.com/jayalane/go-tinyconfig"
	treewalk "github.com/jayalane/go-treewalk"
	_ "net/http/pprof"
	"os"
	"strings"
	"sync"
)

var ml lll.Lll

var theConfig *config.Config
var defaultConfig = `#
cwd = .
debugLevel = network
# comments
`

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

	// low level logging (first so everything rotates)
	ml = lll.Init("SEARCH", (*theConfig)["debugLevel"].StrVal)

	theDir := (*theConfig)["cwd"].StrVal
	app := treewalk.New(theDir, 2)
	app.SetHandler(1, // files
		func(sp treewalk.StringPath, chList []chan treewalk.StringPath, wg *sync.WaitGroup) {
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
}
