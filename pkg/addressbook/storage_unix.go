//go:build !windows

package addressbook

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
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

	// Acquire exclusive lock (blocking)
	if err := syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX); err != nil {
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
	// Release the lock - ignore errors as we're in cleanup
	_ = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN)
	lockFile.Close()
}
