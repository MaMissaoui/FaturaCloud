//go:build !unix

package main

// restrictFileModes is a no-op where there is no umask; see umask_unix.go.
func restrictFileModes() {}
