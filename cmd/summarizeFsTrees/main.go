// -*- tab-width: 2 -*-

// Command go_summarize_fs_trees summarizes a filesystem tree by hashing
// every file's contents and rolling those hashes (and sizes) up into a
// recursive per-directory content hash.  Two trees can then be compared
// to find directories whose entire contents already exist on the other
// side -- e.g. to decide which directories on a pricey remote backup can
// be deleted because /big already has them.
//
// There are no command-line flags: everything is driven from config.txt
// (read from the directory containing the binary).  Run the binary with
// --dumpConfig to print the default, fully-commented config.txt.
//
// The walk reuses the goroutine-pool tree walker from
// github.com/jayalane/go-treewalk (layer 0 = directories that recurse,
// layer 1 = files that get hashed) exactly like cmd/dupDropbox.
package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	_ "net/http/pprof" //nolint:gosec
	"os"
	"strconv"
	"strings"
	"sync"

	count "github.com/jayalane/go-counter"
	globals "github.com/jayalane/go-globals"
	treewalk "github.com/jayalane/go-treewalk"
)

var g globals.Global

const (
	// depth is the number of treewalk layers: layer 0 directories (which
	// recurse onto themselves) and layer 1 files.
	depth = 2

	// exitErr is the process exit code for a fatal error.
	exitErr = 1

	// recChanBuffer sizes the file-result channel feeding the collector.
	recChanBuffer = 4096
)

// defaultConfig is also what --dumpConfig prints (globals prepends the
// logStdout line).  Keep it heavily commented: it is the only docs a
// user editing config.txt sees.  Full-line comments start with '#' in
// column 0; inline comments use '//'.
var defaultConfig = `#
# go_summarize_fs_trees configuration.  There are no command-line flags;
# every option lives here.  config.txt is read from the directory that
# holds the binary (NOT the current directory).  Run
#   ./go_summarize_fs_trees --dumpConfig
# to print this default with every option explained.
#
# Each option below is tagged with the mode(s) that actually read it.
#
# ===================  MODE  =========================================
# mode = what to do.  One of:
#   summarize     walk 'root', hash every file, write a summary to 'outFile'.
#   compare       read 'haveFile' and 'candidateFile' (two summaries from
#                 earlier summarize runs) and list the directories in the
#                 candidate whose contents already exist on the have side.
#   compare-files like compare, but ALSO lists loose duplicate files (ones
#                 not inside a fully-duplicated dir).  Needs both summaries
#                 to have been made with emitFiles = true.
#   both          one run: summarize 'root'->'outFile' and 'root2'->'out2File',
#                 then compare them.  Use this when both trees are reachable
#                 locally (e.g. the remote volume is mounted here).
#   both-files    like both, but uses compare-files (dirs + loose files).
#                 Per-file hashes are kept in memory automatically; set
#                 emitFiles = true too if you also want them in the JSON.
#   selfdups      scan 'root' and find directories duplicated ELSEWHERE in
#                 the SAME tree (highest level), to delete redundant copies
#                 -- e.g. two rsync runs that landed the same data twice.
#   selfdups-file like selfdups but reads an existing summary ('haveFile')
#                 instead of rescanning -- instant, but only as current as
#                 that summary.
mode = both
#
# ===================  PATHS  ========================================
# root      directory to summarize / the KEEP side.       [summarize, both]
root = /big
# outFile   where root's summary JSON is written.         [summarize, both]
outFile = big.json
#
# root2     the CANDIDATE side you might delete from,                [both]
#           e.g. a local mount of the remote volume.
root2 = /mnt/remote
# out2File  where root2's summary JSON is written.                   [both]
out2File = remote.json
#
# haveFile       summary JSON of the KEEP side.                   [compare]
haveFile = big.json
# candidateFile  summary JSON of the side you might delete.       [compare]
candidateFile = remote.json
#
# ===================  OUTPUT  =======================================
# topN     how many directories to print, largest first.       [all modes]
topN = 50
# minSize  omit/ignore directories smaller than this many       [all modes]
#          bytes (0 = keep everything).
minSize = 0
# deleteScript  also write a runnable shell script of 'rm -rI <dir>'
#               lines, one per reclaimable directory (paths quoted; -rI
#               asks once for confirmation before each recursive delete).
#               Lists ALL reclaimable dirs, not just topN.  Empty string
#               = don't write a script.                  [compare, both]
deleteScript = could_be_deleted_dangerous.sh
# emitFiles  also write every file's md5 into the summary, so a later
#            'compare-files' run can find loose duplicate files.  Makes
#            the JSON much larger; off by default.        [summarize, both]
emitFiles = false
#
# ===================  WALK TUNING  ==================================
# skipDirList  directory names to skip entirely, '|'-separated.
#                                                          [summarize, both]
skipDirList = .snapshot|.git
# numWorkers   goroutines per treewalk layer, as directories,files.
#              On an SSD, raise the file count to keep more reads in
#              flight (deeper queue = more bandwidth); 50MB/s at shallow
#              queue means metadata-bound -- push these up.  On a single
#              spinning disk, LOWER them instead to avoid seek thrashing.
#                                                          [summarize, both]
numWorkers = 32,128
#
# ===================  LOGGING / PROFILING  ==========================
# debugLevel  log verbosity: none | state | network | all.
debugLevel = network
# profListen  address for the net/http/pprof profiling server.
profListen = localhost:8002
# comments
`

// cfgStr returns a trimmed string config value.
func cfgStr(key string) string {
	return strings.TrimSpace((*g.Cfg)[key].StrVal)
}

// cfgInt returns an integer config value (0 if unset/unparseable).
func cfgInt(key string) int {
	n, err := strconv.Atoi(cfgStr(key))
	if err != nil {
		return 0
	}

	return n
}

// cfgBool returns a boolean config value (tinyconfig parses true/false).
func cfgBool(key string) bool {
	return (*g.Cfg)[key].BoolVal
}

// modeNeedsFiles reports whether the configured mode compares on per-file
// hashes in-process (so doSummarize must keep them even if emitFiles is
// off and they never reach disk).
func modeNeedsFiles() bool {
	switch cfgStr("mode") {
	case "both-files", "bf":
		return true
	default:
		return false
	}
}

func main() {
	// Handle --dumpConfig ourselves: globals also handles it, but only
	// when started with the file CPU profiler (doProf=true).  We pass
	// doProf=false so the live /debug/pprof CPU endpoint works, and in
	// that mode globals' own --dumpConfig path dereferences a nil
	// profiler and panics -- so intercept it here first.
	if len(os.Args) > 1 && os.Args[1] == "--dumpConfig" {
		fmt.Println("logStdout = false\n" + defaultConfig)

		return
	}

	// globals reads config.txt and sets up logging/counters.  doProf is
	// false on purpose: the file profiler would hold the single CPU
	// profile slot and block the live pprof endpoint below.
	g = globals.NewGlobal(defaultConfig, false)

	// start the profiler
	go func() {
		if len((*g.Cfg)["profListen"].StrVal) > 0 {
			g.Ml.La(http.ListenAndServe((*g.Cfg)["profListen"].StrVal, nil))
		}
	}()

	dispatch()
}

// dispatch runs the operation named by the mode config key.
func dispatch() {
	switch cfgStr("mode") {
	case "summarize", "scan":
		doSummarize(cfgStr("root"), cfgStr("outFile"))
	case "compare", "diff":
		doCompare(loadOrDie("haveFile", cfgStr("haveFile")),
			loadOrDie("candidateFile", cfgStr("candidateFile")))
	case "compare-files", "cf":
		doCompareFiles(loadOrDie("haveFile", cfgStr("haveFile")),
			loadOrDie("candidateFile", cfgStr("candidateFile")))
	case "both", "all":
		have := doSummarize(cfgStr("root"), cfgStr("outFile"))
		cand := doSummarize(cfgStr("root2"), cfgStr("out2File"))
		doCompare(have, cand)
	case "both-files", "bf":
		have := doSummarize(cfgStr("root"), cfgStr("outFile"))
		cand := doSummarize(cfgStr("root2"), cfgStr("out2File"))
		doCompareFiles(have, cand)
	case "selfdups", "sd":
		reportSelfDups(doSummarize(cfgStr("root"), cfgStr("outFile")))
	case "selfdups-file", "sdf":
		reportSelfDups(loadOrDie("haveFile", cfgStr("haveFile")))
	default:
		g.Ml.La("Unknown mode", cfgStr("mode"),
			"- expected summarize, compare, compare-files, both, both-files,",
			"selfdups or selfdups-file")
		os.Exit(exitErr)
	}
}

// parseNumWorkers turns a config line '20,40' into a slice of ints
// [20, 40], one entry per treewalk layer.
func parseNumWorkers(sNums []string, depth int) []int64 {
	if len(sNums) != depth {
		s := fmt.Sprintln("misconfigured numWorkers", (*g.Cfg)["numWorkers"])
		panic(s)
	}

	gNums := make([]int64, depth)

	for i, k := range sNums {
		n, err := strconv.Atoi(strings.TrimSpace(k))
		if err != nil {
			s := fmt.Sprintln("misconfigured numWorkers", (*g.Cfg)["numWorkers"], k, err)
			panic(s)
		}

		gNums[i] = int64(n)
	}

	return gNums
}

func parseSkipDirs(str string) []string {
	splits := strings.Split(str, "|")
	res := make([]string, len(splits))
	copy(res, splits)

	return res
}

// fileHandler is the layer-1 callback: open and hash one regular file,
// then send its record up the results channel.
func fileHandler(sp treewalk.StringPath, out chan<- fileRec) {
	fullPath := append(sp.Path, sp.Name) //nolint:gocritic
	fn := strings.Join(fullPath, "/")

	// treewalk's Readdir already Lstat'd this entry and passes the
	// FileInfo in sp.Value; reuse it instead of paying for a second stat
	// syscall per file (the big metadata cost when walking many small
	// files).  Fall back to Lstat only if it is somehow missing.
	fi, ok := sp.Value.(os.FileInfo)
	if !ok {
		var err error

		fi, err = os.Lstat(fn)
		if err != nil {
			count.IncrSuffix("file-handler-stat-err", "handler")
			g.Ml.La("Error on stat", fn, err)

			return
		}
	}

	if !fi.Mode().IsRegular() {
		count.IncrSuffix("file-handler-skip-not-regular", "handler")
		g.Ml.Ln("Skipping file is not regular", fn)

		return
	}

	f, err := os.Open(fn)
	if err != nil {
		count.IncrSuffix("file-handler-open-err", "handler")
		g.Ml.La("Error opening", fn, err)

		return
	}

	hash, err := hashReadCloser(f)
	if err != nil {
		count.IncrSuffix("file-handler-hash-read-err", "handler")
		g.Ml.La("Error reading for hash", fn, err)

		return
	}

	out <- fileRec{path: fn, size: fi.Size(), hash: hash}

	count.IncrSuffix("file-handler-ok", "handler")
}

// doSummarize walks root, hashes every file, rolls the results up into a
// per-directory summary, writes it to outFile, prints the largest dirs,
// and returns the summary (so 'both' mode can compare without re-reading).
func doSummarize(root, outFile string) summary {
	root = strings.TrimRight(root, "/")
	if root == "" {
		root = "/"
	}

	g.Ml.La("Summarizing", root, "->", outFile)

	app := treewalk.New(root, depth)
	app.SetNumWorkers(parseNumWorkers(strings.Split(cfgStr("numWorkers"), ","), depth))
	app.SetSkipDirs(parseSkipDirs(cfgStr("skipDirList")))
	app.SetLogLevel(cfgStr("debugLevel"))

	// collector: a single goroutine drains the results channel so the
	// file workers never block on a shared slice.
	recCh := make(chan fileRec, recChanBuffer)

	var (
		recs []fileRec
		cwg  sync.WaitGroup
	)

	cwg.Add(1)

	go func() {
		defer cwg.Done()

		for r := range recCh {
			recs = append(recs, r)
		}
	}()

	app.SetHandler(1, func(sp treewalk.StringPath) {
		fileHandler(sp, recCh)
	})

	app.Start()
	app.Wait()

	close(recCh)
	cwg.Wait()

	count.LogCounters()
	g.Ml.La("Hashed", len(recs), "files; rolling up directories")

	sum := rollup(root, recs)

	// Record every file's md5 when we will need it: either emitFiles asks
	// to persist them, or the in-process both-files mode will compare on
	// them.  Building the slice is cheap; whether it reaches disk is a
	// separate decision (emitFiles), made in writeSummary.
	if cfgBool("emitFiles") || modeNeedsFiles() {
		sum.Files = make([]fileSummary, 0, len(recs))

		for _, r := range recs {
			sum.Files = append(sum.Files, fileSummary{
				Path: r.path,
				Rel:  relPath(root, r.path),
				Size: r.size,
				Hash: r.hash,
			})
		}
	}

	if err := writeSummary(outFile, sum, int64(cfgInt("minSize")), cfgBool("emitFiles")); err != nil {
		g.Ml.La("Error writing output", outFile, err)
		os.Exit(exitErr)
	}

	g.Ml.La("Wrote", outFile, "with", len(sum.Dirs), "directories")
	printTop(sum, cfgInt("topN"))

	return sum
}

// writeSummary writes sum to path as indented JSON, dropping dirs below
// minSize.  Per-file records are written only when emitFiles is set (they
// may be in memory regardless, e.g. for both-files mode).
func writeSummary(path string, sum summary, minSize int64, emitFiles bool) error {
	if !emitFiles {
		sum.Files = nil // keep them in memory for the caller, off disk
	}

	if minSize > 0 {
		// fresh slice -- do NOT reuse sum.Dirs' backing array, or the
		// caller's returned summary (shared backing) would be corrupted.
		kept := make([]dirSummary, 0, len(sum.Dirs))

		for _, d := range sum.Dirs {
			if d.Size >= minSize {
				kept = append(kept, d)
			}
		}

		sum.Dirs = kept
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")

	return enc.Encode(sum)
}

func printTop(sum summary, top int) {
	fmt.Printf("\nLargest directories under %s:\n\n", sum.Root)
	fmt.Printf("%12s  %9s  %s\n", "SIZE", "FILES", "PATH")

	n := top
	if n > len(sum.Dirs) {
		n = len(sum.Dirs)
	}

	for _, d := range sum.Dirs[:n] {
		fmt.Printf("%12s  %9d  %s\n", humanize(d.Size), d.Files, d.Path)
	}
}
