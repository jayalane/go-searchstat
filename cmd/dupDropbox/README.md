Dedup
===========

This builds a binary that checks two directorys and lists the
files that are the same in both. 

So create a config.txt lnnike this:

```
cwd1 = ./Dropbox
cwd2 = ./Dropbox.old
```

and run the binary.

It uses the scaffolding in https://github.com/jayalane/go-treewalk to
do the tree walking, (and timeout-able ReadDir) and uses
https://github.com/jayalane/go-dedup-map to calculate the dupes.





