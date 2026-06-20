# go_summarize_fs_trees

Summarize a filesystem tree by content, then compare two summaries to find
directories that are fully duplicated across them — so you can reclaim space.

The motivating use case: you have ~600 GB on a paid remote volume that is
largely a copy of data already under `/big` on this machine. This tool tells
you **which directories on the remote can be deleted** because their entire
contents already exist locally, ranked by how much space each one frees.

## How it works

The walk uses a pool of goroutines (the
[`go-treewalk`](https://github.com/jayalane/go-treewalk) dataflow walker used
by `cmd/dupDropbox`: layer 0 = directories that recurse, layer 1 = files).
Every regular file's contents are hashed (sha256). After the walk, file
hashes and sizes are rolled up **post-order** into a recursive per-directory
hash:

```
dirHash(D) = sha256(
  for each file child  f, sorted:  "F <name> <size> <filehash>\n"
  for each subdir child s, sorted:  "D <name> <size> <dirHash(s)>\n"
)
```

Because the hash covers child *names*, structure, and file *contents* — but
**not** the directory's own name or absolute path — two directories produce
the same hash exactly when their whole subtrees are byte-identical, no matter
where they live. That is what lets `/big/photos` match
`/mnt/remote/bkp/photos`.

Comparing two summaries finds every candidate directory whose hash exists on
the keep side and reports the **maximal** matches (if a parent directory also
matches, the children are folded into it so space is never double-counted),
largest first.

> Note: empty directories and zero-byte-only trees are ignored — they free no
> space. Hashing is sha256, not md5, so distinct files never collide into a
> false duplicate.

## Configuration — no command-line flags

Everything is driven from **`config.txt`**, which lives next to the binary
(it is read from the directory containing the executable, not the current
directory). Run the binary with the single diagnostic flag `--dumpConfig` to
print the built-in, fully-commented default config:

```sh
./go_summarize_fs_trees --dumpConfig > config.txt   # then edit config.txt
```

The `mode` key selects what happens:

| `mode` | what it does | keys it uses |
|--------|--------------|--------------|
| `summarize` | walk `root`, hash files, write `outFile` | `root`, `outFile` |
| `compare` | read `haveFile` + `candidateFile`, list duplicated dirs | `haveFile`, `candidateFile` |
| `both` | summarize `root`→`outFile` and `root2`→`out2File`, then compare them | all of the above + `root2`, `out2File` |

Shared keys: `topN` (how many dirs to print), `minSize` (ignore/omit dirs
below this many bytes), `skipDirList`, `numWorkers`, `debugLevel`,
`profListen`. See `--dumpConfig` for the full annotated list.

## Typical workflow

Because the remote machine usually can't be walked from here, run `summarize`
on each side and compare the two JSON files locally:

```sh
# 1. on this machine, config.txt has: mode=summarize, root=/big, outFile=big.json
./go_summarize_fs_trees

# 2. on the remote box (copy the binary + a config.txt with
#    mode=summarize, root=/, outFile=remote.json over), then run it and
#    copy remote.json back here.

# 3. here, set config.txt to:
#    mode=compare, haveFile=big.json, candidateFile=remote.json
./go_summarize_fs_trees
```

If the remote volume is mounted locally (e.g. at `/mnt/remote`) you can do it
all in one run with `mode=both`, `root=/big`, `root2=/mnt/remote`.

Example compare output:

```
Removable directories (content already present on the keep side), largest first:

        SIZE      FILES  CANDIDATE DIR  ->  ALREADY AT
      48.2GiB      9123  /mnt/remote/photos/2019  ->  /big/photos/2019
      31.0GiB      4001  /mnt/remote/music        ->  /big/media/music
...
Total reclaimable: 312.7GiB across 27 directories.
```

The summary JSON is also useful on its own — it lists every directory with its
content hash, total size, and file count (sorted largest first).

## Notes / limitations

- The summarize pass holds one small record (path, size, hash) per file in
  memory before the rollup. For a few million files that is a few hundred MB;
  fine for a 600 GB tree.
- Matching is whole-subtree: a directory is reported only when *all* of its
  content exists on the keep side. Partially-overlapping directories are not
  reported (their fully-matching subdirectories are).
