// -*- tab-width:2 -*-

// I vaguely remember reading something about how fast grep is because
// of not doing this naive algorithm but hopefully this is fast enough.
// maybe Contains does the right grep thing.

package main

import (
	"bufio"
	"fmt"
	count "github.com/jayalane/go-counter"
	"io"
	"strings"
)

var shells = []string{
	"bin/ksh",
	"bin/sh",
	"bin/bash",
	"env",
}

var shellExtensions = []string{
	".ksh",
	".sh",
	".env",
	".bash",
	".profile",
	".distro",
}

func anyContains(line string, things []string) bool {
	for _, thing := range things {
		if strings.Contains(line, thing) {
			return true
		}
	}
	return false
}

func anySuffix(line string, things []string) bool {
	for _, thing := range things {
		if strings.HasSuffix(line, thing) {
			return true
		}
	}
	return false
}

func hasShellExtension(fn string) bool {
	periods := strings.Split(fn, ".")
	if len(periods) == 1 {
		return false
	}
	return anySuffix(
		periods[len(periods)-1],
		shellExtensions)
}

func findString(fn string, a io.ReadCloser, m string) (bool, error) {
	defer a.Close()
	shLen := 0
	shNumLines := 0
	once := false
	scanner := bufio.NewScanner(a)
	for scanner.Scan() {
		line := scanner.Text()
		if err := scanner.Err(); err != nil {
			if err == io.EOF {
				break
			}
			ml.La("Error opening file config", err)
			return false, err
		}
		if !once {
			once = true
			if !anyContains(line, shells) && !hasShellExtension(fn) {
				count.IncrSuffix("grep-shell-for-git-not-shell", "grep")
				return false, nil // not a shell, don't check anything
			}
		}
		shLen += len(line)
		shNumLines++
		if len(line) > 0 && line[:1] == "#" { // TODO  space space #
			continue
		}
		if gitRE.MatchString(line) {
			fmt.Println(line)
			count.MarkDistributionSuffix("grep-shell-sh-len", float64(shLen), "grep")
			count.MarkDistributionSuffix("grep-shell-sh-num-lines", float64(shNumLines), "grep")
			return true, nil
		}
	}
	count.MarkDistributionSuffix("grep-shell-sh-len", float64(shLen), "grep")
	count.MarkDistributionSuffix("grep-shell-sh-num-lines", float64(shNumLines), "grep")
	return false, nil
}
