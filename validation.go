package glueberry

import (
	"fmt"
	"unicode"
)

// ValidateStreamName checks if the stream name is valid.
// Stream names must:
//   - Be non-empty
//   - Contain only alphanumeric characters, hyphens, and underscores
//   - Not exceed the maximum length (if maxLength > 0)
//
// Returns nil if valid, or an error describing the validation failure.
func ValidateStreamName(name string, maxLength int) error {
	if name == "" {
		return fmt.Errorf("%w: stream name cannot be empty", ErrInvalidStreamName)
	}

	if maxLength > 0 && len(name) > maxLength {
		return fmt.Errorf("%w: %d characters exceeds maximum of %d",
			ErrStreamNameTooLong, len(name), maxLength)
	}

	for i, r := range name {
		if !isValidStreamNameChar(r) {
			return fmt.Errorf("%w: invalid character %q at position %d (only alphanumeric, hyphen, and underscore allowed)",
				ErrInvalidStreamName, r, i)
		}
	}

	return nil
}

// isValidStreamNameChar returns true if the rune is valid in a stream name.
// Valid characters are: a-z, A-Z, 0-9, hyphen (-), underscore (_)
func isValidStreamNameChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_'
}

// ValidateStreamNames validates a slice of stream names.
// Returns an error if any name is invalid.
func ValidateStreamNames(names []string, maxLength int) error {
	if len(names) == 0 {
		return ErrNoStreamsRequested
	}

	for _, name := range names {
		if err := ValidateStreamName(name, maxLength); err != nil {
			return err
		}
	}

	return nil
}

// ValidateMetadataSize checks if the total metadata size is within limits.
// Returns nil if valid, or ErrMetadataTooLarge if the total size exceeds maxSize.
// Size is calculated as the sum of key and value lengths.
func ValidateMetadataSize(metadata map[string]string, maxSize int) error {
	if maxSize <= 0 || metadata == nil {
		return nil
	}

	var totalSize int
	for k, v := range metadata {
		totalSize += len(k) + len(v)
	}

	if totalSize > maxSize {
		return fmt.Errorf("%w: %d bytes exceeds maximum of %d bytes",
			ErrMetadataTooLarge, totalSize, maxSize)
	}

	return nil
}
