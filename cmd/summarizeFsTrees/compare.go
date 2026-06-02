// -*- tab-width:2 -*-

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// loadSummary reads a summary JSON document written by `summarize`.
func loadSummary(path string) (summary, error) {
	var s summary

	b, err := os.ReadFile(path)
	if err != nil {
		return s, err
	}

	if err := json.Unmarshal(b, &s); err != nil {
		return s, err
	}

	return s, nil
}

// loadOrDie loads a summary or prints an error and exits.
func loadOrDie(flagName, path string) summary {
	s, err := loadSummary(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading %s (%s): %v\n", flagName, path, err)
		os.Exit(exitErr)
	}

	return s
}

// ancestors returns the relative paths that strictly contain rel, from
// the root (".") down to rel's parent.  rel "." has no ancestors.
func ancestors(rel string) []string {
	if rel == "." || rel == "" {
		return nil
	}

	segs := strings.Split(rel, "/")
	res := []string{"."}

	for i := 1; i < len(segs); i++ {
		res = append(res, strings.Join(segs[:i], "/"))
	}

	return res
}

type reclaim struct {
	cand     dirSummary
	havePath string
}

// isCovered reports whether some ancestor of rel is also removable, in
// which case deleting that ancestor already reclaims rel.
func isCovered(rel string, removable map[string]struct{}) bool {
	for _, a := range ancestors(rel) {
		if _, ok := removable[a]; ok {
			return true
		}
	}

	return false
}

// findReclaims returns the maximal candidate directories whose entire
// content already lives on the keep side (largest first).
func findReclaims(have, cand summary, minSize int64) []reclaim {
	// hash -> an example path on the keep side
	haveHashes := make(map[string]string, len(have.Dirs))
	for _, d := range have.Dirs {
		if _, ok := haveHashes[d.Hash]; !ok {
			haveHashes[d.Hash] = d.Path
		}
	}

	// every candidate dir whose whole subtree exists on the keep side
	removable := make(map[string]struct{})
	hits := []reclaim{}

	for _, d := range cand.Dirs {
		hp, ok := haveHashes[d.Hash]
		if !ok || d.Size < minSize {
			continue
		}

		removable[d.Rel] = struct{}{}

		hits = append(hits, reclaim{cand: d, havePath: hp})
	}

	// keep only maximal matches (drop dirs covered by a removable ancestor)
	maximal := []reclaim{}

	for _, h := range hits {
		if !isCovered(h.cand.Rel, removable) {
			maximal = append(maximal, h)
		}
	}

	sort.Slice(maximal, func(i, j int) bool {
		return maximal[i].cand.Size > maximal[j].cand.Size
	})

	return maximal
}

// doCompare finds directories in the candidate tree whose content is
// already present in the have tree, and reports the largest removable
// ones (with no double counting of nested matches).  topN and minSize
// come from config.txt.
func doCompare(have, cand summary) {
	maximal := findReclaims(have, cand, int64(cfgInt("minSize")))

	printReclaim(cand.Root, have.Root, maximal, cfgInt("topN"))
}

func printReclaim(candRoot, haveRoot string, maximal []reclaim, top int) {
	var total int64
	for _, h := range maximal {
		total += h.cand.Size
	}

	fmt.Printf("Candidate tree: %s\n", candRoot)
	fmt.Printf("Compared against: %s\n\n", haveRoot)

	if len(maximal) == 0 {
		fmt.Println("No directories in the candidate tree are fully duplicated on the keep side.")

		return
	}

	fmt.Printf("Removable directories (content already present on the keep side), largest first:\n\n")
	fmt.Printf("%12s  %9s  %s\n", "SIZE", "FILES", "CANDIDATE DIR  ->  ALREADY AT")

	shown := maximal
	if len(shown) > top {
		shown = shown[:top]
	}

	for _, h := range shown {
		fmt.Printf("%12s  %9d  %s  ->  %s\n",
			humanize(h.cand.Size), h.cand.Files, h.cand.Path, h.havePath)
	}

	if len(maximal) > len(shown) {
		fmt.Printf("\n... and %d more not shown (raise -top to see them).\n", len(maximal)-len(shown))
	}

	fmt.Printf("\nTotal reclaimable: %s across %d directories.\n", humanize(total), len(maximal))
}
