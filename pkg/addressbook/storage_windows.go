//go:build windows

package addressbook

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// acquireFileLock acquires an exclusive file lock for inter-process synchronization.
// Returns the lock file handle which must be passed to releaseFileLock.
func (s *storage) acquireFileLock() (*os.File, error) {
	// Ensure the directory exists
	dir := filepath.Dir(s.lockPath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create lock directory: %w", err)
		}
	}

	// Open or create the lock file
	lockFile, err := os.OpenFile(s.lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to open lock file: %w", err)
	}

	// Acquire exclusive lock using Windows LockFileEx
	// LOCKFILE_EXCLUSIVE_LOCK = 0x2, LOCKFILE_FAIL_IMMEDIATELY = 0x1
	// We use only EXCLUSIVE_LOCK (0x2) to block until lock is acquired
	var overlapped windows.Overlapped
	err = windows.LockFileEx(
		windows.Handle(lockFile.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK,
		0,          // reserved, must be zero
		1,          // lock 1 byte (minimum)
		0,          // high-order DWORD of byte range
		&overlapped,
	)
	if err != nil {
		lockFile.Close()
		return nil, fmt.Errorf("failed to acquire file lock: %w", err)
	}

	return lockFile, nil
}

// releaseFileLock releases the file lock and closes the lock file.
func (s *storage) releaseFileLock(lockFile *os.File) {
	if lockFile == nil {
		return
	}

	// Release the lock using Windows UnlockFileEx - ignore errors as we're in cleanup
	var overlapped windows.Overlapped
	_ = windows.UnlockFileEx(
		windows.Handle(lockFile.Fd()),
		0, // reserved
		1, // unlock 1 byte
		0, // high-order DWORD
		&overlapped,
	)
	lockFile.Close()
}
