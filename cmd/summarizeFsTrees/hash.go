// -*- tab-width:2 -*-

package main

import (
	"crypto/md5" //nolint:gosec // md5 is fine for non-adversarial dedup and faster than sha256
	"encoding/hex"
	"io"
	"sync"
)

// hashBufSize is the read buffer used per file.  A big buffer cuts the
// number of read syscalls ~32x versus io.Copy's default 32 KiB, which
// is the main win when reading from a network / remote filesystem.
const hashBufSize = 1 << 20 // 1 MiB

// hashBufPool recycles the per-file read buffers so hashing millions of
// files doesn't allocate (and GC) a megabyte each time.
var hashBufPool = sync.Pool{
	New: func() any {
		b := make([]byte, hashBufSize)

		return &b
	},
}

// hashReadCloser reads all of a and returns the hex md5 of its contents,
// always closing a.  md5 (rather than sha256) is plenty to tell files
// apart for de-duplication in a non-adversarial setting, and is much
// faster on CPUs without SHA hardware acceleration.
func hashReadCloser(a io.ReadCloser) (string, error) {
	defer a.Close()

	aHash := md5.New() //nolint:gosec // see import note

	bufp, _ := hashBufPool.Get().(*[]byte)
	defer hashBufPool.Put(bufp)

	if _, err := io.CopyBuffer(aHash, a, *bufp); err != nil {
		return "", err
	}

	return hex.EncodeToString(aHash.Sum(nil)), nil
}
