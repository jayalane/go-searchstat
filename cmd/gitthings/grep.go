// -*- tab-width:2 -*-

// I vaguely remember reading something about how fast grep is because
// of not doing this naive algorithm but hopefully this is fast enough.
// maybe Contains does the right grep thing.

package main

import (
	"bufio"
	"fmt"
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
				return false, nil // not a shell, don't check anything
			}
		}
		if len(line) > 0 && line[:1] == "#" { // TODO  space space #
			continue
		}
		if gitRE.MatchString(line) {
			fmt.Println(line)
			return true, nil
		}
	}
	return false, nil
}
