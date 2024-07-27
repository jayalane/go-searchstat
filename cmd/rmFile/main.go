// -*- tab-width: 2 -*-

package main

import (
	"bufio"
	"errors"
	"io"
	_ "net/http/pprof" //nolint:gosec
	"os"

	count "github.com/jayalane/go-counter"
	globals "github.com/jayalane/go-globals"
)

var g globals.Global

var defaultConfig = `#
fileToRm = .
debugLevel = network
profListen = localhost:8002
doubleCheck = no
# comments
`

func lineHandler(fn string) {
	g.Ml.La("Checking for deletion {", fn, "}")

	fi, err := os.Lstat(fn)
	if err != nil {
		count.IncrSyncSuffix("file-handler-stat-err", "handler")
		g.Ml.La("Error on stat", fn, err)

		return
	}

	if !fi.Mode().IsRegular() {
		count.IncrSyncSuffix("fileb-handler-skip-not-regular", "handler")
		g.Ml.La("Skipping file is not regular", fn)

		return
	}

	if (*g.Cfg)["doubleCheck"].StrVal == "danger" {
		err := os.Remove(fn)
		if err != nil {
			count.IncrSyncSuffix("fileb-handler-rm-err", "handler")
			g.Ml.La("Error opening", fn, err)

			return
		}
	}

	count.IncrSyncSuffix("fileb-handler-rm-ok", "handler")
}

func handleFile(theFile string) {
	g.Ml.La("Reading file deletion list", theFile)

	file, err := os.Open(theFile)
	if err != nil {
		if os.IsNotExist(err) {
			g.Ml.La(theFile, "does not exist, exiting")

			return
		}

		g.Ml.La("Warning: can't open file deletion list, exiting,", theFile, err.Error())

		return
	}

	defer file.Close()

	fileReader := bufio.NewReader(file)

	scanner := bufio.NewScanner(fileReader)
	for scanner.Scan() {
		line := scanner.Text()

		if err := scanner.Err(); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}

			g.Ml.La("Error reading file deletion list", err)

			return
		}

		if len(line) > 0 && line[:1] == "#" { // later space then space #
			continue
		}

		lineHandler(line)
	}
}

func main() {
	g = globals.NewGlobal(defaultConfig, true)

	// first start directory
	theFile := (*g.Cfg)["fileToRm"].StrVal

	handleFile(theFile)

	count.LogCounters()
}
