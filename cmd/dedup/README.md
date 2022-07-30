Dedup
===========

This builds a binary that prints the all the files with sha256 being
the same as other files under the directory specified by the cwd in
the config.txt file.

So create a config.txt lnnike this:

```
cwd = .
```

and run the binary.

It uses the scaffolding in https://github.com/jayalane/go-treewalk to
do the tree walking, (and timeout-able ReadDir) and uses
https://github.com/jayalane/go-dedup-map to calculate the dupes.





