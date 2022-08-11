// -*- tab-width:2 -*-

// I vaguely remember reading something about how fast grep is because
// of not doing this naive algorithm but hopefully this is fast enough.
// maybe Contains does the right grep thing.

package main

import (
	"bufio"
	"fmt"
	"io"
)

func findString(a io.ReadCloser, m string) (bool, error) {
	defer a.Close()
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
