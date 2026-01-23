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
)

// storage handles file persistence for the address book.
type storage struct {
	path string
	mu   sync.Mutex
}

// newStorage creates a new storage instance for the given file path.
func newStorage(path string) *storage {
	return &storage{path: path}
}

// load reads the address book from disk.
// Returns an empty data structure if the file doesn't exist.
// If the file is corrupted, it creates a backup and returns empty data.
func (s *storage) load() (*addressBookData, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

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
// It writes to a temporary file first, then renames to the target path.
func (s *storage) save(book *addressBookData) error {
	s.mu.Lock()
	defer s.mu.Unlock()

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
	if err := os.WriteFile(tempPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write temporary file: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tempPath, s.path); err != nil {
		// Clean up temp file on failure
		os.Remove(tempPath)
		return fmt.Errorf("failed to rename temporary file: %w", err)
	}

	return nil
}

// exists checks if the address book file exists.
func (s *storage) exists() bool {
	_, err := os.Stat(s.path)
	return err == nil
}

// delete removes the address book file.
func (s *storage) delete() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.Remove(s.path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete address book: %w", err)
	}
	return nil
}
