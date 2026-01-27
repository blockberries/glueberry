package addressbook

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const (
	// currentVersion is the current address book format version.
	currentVersion = 1

	// tempFileSuffix is appended to the file path for atomic writes.
	tempFileSuffix = ".tmp"

	// backupFileSuffix is appended when backing up corrupted files.
	backupFileSuffix = ".bak"

	// lockFileSuffix is appended to create a lock file for inter-process synchronization.
	lockFileSuffix = ".lock"
)

// storage handles file persistence for the address book.
type storage struct {
	path     string
	lockPath string
	mu       sync.Mutex
}

// newStorage creates a new storage instance for the given file path.
func newStorage(path string) *storage {
	return &storage{
		path:     path,
		lockPath: path + lockFileSuffix,
	}
}

// load reads the address book from disk.
// Returns an empty data structure if the file doesn't exist.
// If the file is corrupted, it creates a backup and returns empty data.
// Uses file locking to prevent concurrent access from multiple processes.
func (s *storage) load() (*addressBookData, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Acquire inter-process file lock
	lockFile, err := s.acquireFileLock()
	if err != nil {
		return nil, fmt.Errorf("failed to acquire lock for load: %w", err)
	}
	defer s.releaseFileLock(lockFile)

	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist, return empty data
			return &addressBookData{
				Version: currentVersion,
				Peers:   make(map[string]*PeerEntry),
			}, nil
		}
		return nil, fmt.Errorf("failed to read address book: %w", err)
	}

	// Handle empty file
	if len(data) == 0 {
		return &addressBookData{
			Version: currentVersion,
			Peers:   make(map[string]*PeerEntry),
		}, nil
	}

	var book addressBookData
	if err := json.Unmarshal(data, &book); err != nil {
		// File is corrupted, backup and return empty
		backupPath := s.path + backupFileSuffix
		if backupErr := os.Rename(s.path, backupPath); backupErr != nil {
			return nil, fmt.Errorf("failed to parse address book and backup failed: parse error: %w, backup error: %v", err, backupErr)
		}
		return &addressBookData{
			Version: currentVersion,
			Peers:   make(map[string]*PeerEntry),
		}, nil
	}

	// Initialize peers map if nil
	if book.Peers == nil {
		book.Peers = make(map[string]*PeerEntry)
	}

	return &book, nil
}

// save writes the address book to disk atomically.
// It writes to a temporary file first, syncs to disk, then renames to the target path.
// Uses file locking to prevent concurrent access from multiple processes.
func (s *storage) save(book *addressBookData) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Acquire inter-process file lock
	lockFile, err := s.acquireFileLock()
	if err != nil {
		return fmt.Errorf("failed to acquire lock for save: %w", err)
	}
	defer s.releaseFileLock(lockFile)

	// Ensure the directory exists
	dir := filepath.Dir(s.path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory: %w", err)
		}
	}

	// Marshal to JSON with indentation for readability
	data, err := json.MarshalIndent(book, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal address book: %w", err)
	}

	// Write to temporary file
	tempPath := s.path + tempFileSuffix
	tempFile, err := os.OpenFile(tempPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}

	if _, err := tempFile.Write(data); err != nil {
		tempFile.Close()
		os.Remove(tempPath)
		return fmt.Errorf("failed to write temporary file: %w", err)
	}

	// Sync to disk to ensure durability before rename
	if err := tempFile.Sync(); err != nil {
		tempFile.Close()
		os.Remove(tempPath)
		return fmt.Errorf("failed to sync temporary file: %w", err)
	}

	if err := tempFile.Close(); err != nil {
		os.Remove(tempPath)
		return fmt.Errorf("failed to close temporary file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tempPath, s.path); err != nil {
		// Clean up temp file on failure
		os.Remove(tempPath)
		return fmt.Errorf("failed to rename temporary file: %w", err)
	}

	return nil
}
