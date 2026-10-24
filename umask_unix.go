//go:build unix

package main

import "syscall"

// restrictFileModes sets the process umask to 0027, so files the server
// creates, the live SQLite database and its -wal/-shm siblings included,
// are 0640 instead of the 0644 the image's default umask gives. SQLite
// recreates -wal and -shm on every start, so a chmod after opening would
// have to run each time; the umask covers every path at once. 0640 rather
// than 0600 for the same reason as db.BackupFileMode: a host ACL granting
// the backup user group read access must keep working (audit F195).
func restrictFileModes() {
	syscall.Umask(0o027)
}
