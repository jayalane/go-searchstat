Dedup
===========

This builds a binary that prints the all the .git dirs and all the
files with "git pull" in them. 

So create a config.txt like this:

```
cwd = .
```

and run the binary.

It uses the scaffolding in https://github.com/jayalane/go-treewalk to
do the tree walking, (and timeout-able ReadDir).  






