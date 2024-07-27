RmEmptyDirs
===========

This builds a binary that removes any empty directories.  It's not recursive
so it would need to be run a few times.  

```
cwd = .
```

and run the binary.

It uses the scaffolding in https://github.com/jayalane/go-treewalk to
setup the go routines and channels, and just specifies a callback to
call Lstat and print if appropriate. 




