Search Stat
===========

This builds a binary that prints the name and last modified time for
all zero-length files under the directory specified by the cwd in the
config.txt file.

So create a config.txt like this:

```
cwd = .
```

and run the binary.

It uses the scaffolding in https://github.com/jayalane/go-treewalk to
setup the go routines and channels, and just specifies a callback to
call Lstat and print if appropriate. 




