// -*- tab-width:2 -*-

package main

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
)

func hashReadCloser(a io.ReadCloser) (string, error) {
	defer a.Close()

	aHash := sha256.New()
	_, err := io.Copy(aHash, a)
	if err != nil {
		return "", err
	}
	aHashStr := hex.EncodeToString(aHash.Sum(nil))
	return aHashStr, nil
}
