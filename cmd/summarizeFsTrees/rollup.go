// -*- tab-width:2 -*-

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// fileRec is one regular file the walk hashed.  path is the absolute
// path, size the byte length, hash the hex sha256 of its contents.
type fileRec struct {
	path string
	size int64
	hash string
}

// dirSummary is one directory's rolled-up result: the recursive
// content hash, total bytes, and file count of everything beneath it.
// Two dirSummaries with the same Hash have byte-identical subtrees
// (same names, same structure, same file contents) regardless of where
// they live, which is what makes cross-machine matching work.
type dirSummary struct {
	Path  string `json:"path"`  // absolute path
	Rel   string `json:"rel"`   // path relative to Root ("." for the root)
	Hash  string `json:"hash"`  // recursive content hash
	Size  int64  `json:"size"`  // total bytes under this dir
	Files int64  `json:"files"` // total regular files under this dir
}

// summary is the whole on-disk document a summarize run produces.
type summary struct {
	Root string       `json:"root"`
	Dirs []dirSummary `json:"dirs"`
}

// dirNode is a node in the in-memory tree we build from the flat list
// of hashed files before rolling sizes/hashes up post-order.
type dirNode struct {
	children map[string]*dirNode
	files    map[string]fileRec

	size      int64
	fileCount int64
	hash      string
}

func newDirNode() *dirNode {
	return &dirNode{
		children: map[string]*dirNode{},
		files:    map[string]fileRec{},
	}
}

func sortedKeys[V any](m map[string]V) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}

	sort.Strings(ks)

	return ks
}

// relPath returns r.path expressed relative to root ("" if equal).
func relPath(root, full string) string {
	rel := strings.TrimPrefix(full, root)
	rel = strings.TrimPrefix(rel, "/")

	return rel
}

// buildTree inserts every file record into a tree rooted at root.
func buildTree(root string, recs []fileRec) *dirNode {
	rootNode := newDirNode()

	for _, r := range recs {
		rel := relPath(root, r.path)
		if rel == "" {
			continue // a file that *is* the root makes no sense; skip
		}

		segs := strings.Split(rel, "/")
		node := rootNode

		for _, d := range segs[:len(segs)-1] {
			child := node.children[d]
			if child == nil {
				child = newDirNode()
				node.children[d] = child
			}

			node = child
		}

		node.files[segs[len(segs)-1]] = r
	}

	return rootNode
}

// compute fills in size, fileCount and hash for n and, recursively,
// all of its descendants (post-order).
func (n *dirNode) compute() {
	var buf strings.Builder

	for _, nm := range sortedKeys(n.files) {
		f := n.files[nm]
		n.size += f.size
		n.fileCount++

		fmt.Fprintf(&buf, "F %s %d %s\n", nm, f.size, f.hash)
	}

	for _, nm := range sortedKeys(n.children) {
		c := n.children[nm]
		c.compute()

		n.size += c.size
		n.fileCount += c.fileCount

		fmt.Fprintf(&buf, "D %s %d %s\n", nm, c.size, c.hash)
	}

	sum := sha256.Sum256([]byte(buf.String()))
	n.hash = hex.EncodeToString(sum[:])
}

// collect flattens the computed tree into dirSummary records.  rel is
// the path of n relative to root ("" for the root itself).
func (n *dirNode) collect(root, rel string, out *[]dirSummary) {
	abs := root

	dispRel := "."

	if rel != "" {
		abs = root + "/" + rel
		dispRel = rel
	}

	*out = append(*out, dirSummary{
		Path:  abs,
		Rel:   dispRel,
		Hash:  n.hash,
		Size:  n.size,
		Files: n.fileCount,
	})

	for _, nm := range sortedKeys(n.children) {
		childRel := nm
		if rel != "" {
			childRel = rel + "/" + nm
		}

		n.children[nm].collect(root, childRel, out)
	}
}

// rollup turns the hashed files into a sorted (largest first) summary.
func rollup(root string, recs []fileRec) summary {
	tree := buildTree(root, recs)
	tree.compute()

	var dirs []dirSummary

	tree.collect(root, "", &dirs)

	sort.Slice(dirs, func(i, j int) bool {
		return dirs[i].Size > dirs[j].Size
	})

	return summary{Root: root, Dirs: dirs}
}

// humanize renders a byte count as a short human-readable string.
func humanize(n int64) string {
	const unit = 1024

	if n < unit {
		return fmt.Sprintf("%dB", n)
	}

	div, exp := int64(unit), 0
	for x := n / unit; x >= unit; x /= unit {
		div *= unit
		exp++
	}

	return fmt.Sprintf("%.1f%ciB", float64(n)/float64(div), "KMGTPE"[exp])
}
