package glueberry

import (
	"errors"
	"testing"
)

func TestValidateStreamName(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		maxLength int
		wantErr   error
	}{
		{
			name:      "valid alphanumeric",
			input:     "mystream123",
			maxLength: 64,
			wantErr:   nil,
		},
		{
			name:      "valid with hyphen",
			input:     "my-stream",
			maxLength: 64,
			wantErr:   nil,
		},
		{
			name:      "valid with underscore",
			input:     "my_stream",
			maxLength: 64,
			wantErr:   nil,
		},
		{
			name:      "valid mixed",
			input:     "My_Stream-123",
			maxLength: 64,
			wantErr:   nil,
		},
		{
			name:      "empty name",
			input:     "",
			maxLength: 64,
			wantErr:   ErrInvalidStreamName,
		},
		{
			name:      "too long",
			input:     "abcdefghijklmnopqrstuvwxyz1234567890",
			maxLength: 10,
			wantErr:   ErrStreamNameTooLong,
		},
		{
			name:      "invalid character space",
			input:     "my stream",
			maxLength: 64,
			wantErr:   ErrInvalidStreamName,
		},
		{
			name:      "invalid character dot",
			input:     "my.stream",
			maxLength: 64,
			wantErr:   ErrInvalidStreamName,
		},
		{
			name:      "invalid character slash",
			input:     "my/stream",
			maxLength: 64,
			wantErr:   ErrInvalidStreamName,
		},
		{
			name:      "invalid character at sign",
			input:     "my@stream",
			maxLength: 64,
			wantErr:   ErrInvalidStreamName,
		},
		{
			name:      "unicode letters allowed",
			input:     "streamé",
			maxLength: 64,
			wantErr:   nil,
		},
		{
			name:      "no max length check if zero",
			input:     "verylongstreamnamethatexceedsdefaultlimit",
			maxLength: 0,
			wantErr:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateStreamName(tt.input, tt.maxLength)
			if tt.wantErr == nil {
				if err != nil {
					t.Errorf("ValidateStreamName(%q, %d) = %v, want nil", tt.input, tt.maxLength, err)
				}
			} else {
				if err == nil {
					t.Errorf("ValidateStreamName(%q, %d) = nil, want error wrapping %v", tt.input, tt.maxLength, tt.wantErr)
				} else if !errors.Is(err, tt.wantErr) {
					t.Errorf("ValidateStreamName(%q, %d) = %v, want error wrapping %v", tt.input, tt.maxLength, err, tt.wantErr)
				}
			}
		})
	}
}

func TestValidateStreamNames(t *testing.T) {
	tests := []struct {
		name      string
		names     []string
		maxLength int
		wantErr   error
	}{
		{
			name:      "valid names",
			names:     []string{"data", "control", "events"},
			maxLength: 64,
			wantErr:   nil,
		},
		{
			name:      "empty slice",
			names:     []string{},
			maxLength: 64,
			wantErr:   ErrNoStreamsRequested,
		},
		{
			name:      "nil slice",
			names:     nil,
			maxLength: 64,
			wantErr:   ErrNoStreamsRequested,
		},
		{
			name:      "one invalid name",
			names:     []string{"valid", "invalid name", "another"},
			maxLength: 64,
			wantErr:   ErrInvalidStreamName,
		},
		{
			name:      "one too long name",
			names:     []string{"short", "verylongname"},
			maxLength: 10,
			wantErr:   ErrStreamNameTooLong,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateStreamNames(tt.names, tt.maxLength)
			if tt.wantErr == nil {
				if err != nil {
					t.Errorf("ValidateStreamNames(%v, %d) = %v, want nil", tt.names, tt.maxLength, err)
				}
			} else {
				if err == nil {
					t.Errorf("ValidateStreamNames(%v, %d) = nil, want error wrapping %v", tt.names, tt.maxLength, tt.wantErr)
				} else if !errors.Is(err, tt.wantErr) {
					t.Errorf("ValidateStreamNames(%v, %d) = %v, want error wrapping %v", tt.names, tt.maxLength, err, tt.wantErr)
				}
			}
		})
	}
}

func TestValidateMetadataSize(t *testing.T) {
	tests := []struct {
		name     string
		metadata map[string]string
		maxSize  int
		wantErr  error
	}{
		{
			name:     "nil metadata",
			metadata: nil,
			maxSize:  1024,
			wantErr:  nil,
		},
		{
			name:     "empty metadata",
			metadata: map[string]string{},
			maxSize:  1024,
			wantErr:  nil,
		},
		{
			name:     "within limit",
			metadata: map[string]string{"key": "value"},
			maxSize:  1024,
			wantErr:  nil,
		},
		{
			name:     "exactly at limit",
			metadata: map[string]string{"key": "val"}, // 3 + 3 = 6 bytes
			maxSize:  6,
			wantErr:  nil,
		},
		{
			name:     "over limit",
			metadata: map[string]string{"key": "value"}, // 3 + 5 = 8 bytes
			maxSize:  5,
			wantErr:  ErrMetadataTooLarge,
		},
		{
			name: "multiple entries over limit",
			metadata: map[string]string{
				"key1": "value1",
				"key2": "value2",
			},
			maxSize: 10, // Each entry is 4+6=10, total 20 bytes
			wantErr: ErrMetadataTooLarge,
		},
		{
			name:     "zero max size disables check",
			metadata: map[string]string{"key": "very long value that would exceed any reasonable limit"},
			maxSize:  0,
			wantErr:  nil,
		},
		{
			name:     "negative max size disables check",
			metadata: map[string]string{"key": "value"},
			maxSize:  -1,
			wantErr:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMetadataSize(tt.metadata, tt.maxSize)
			if tt.wantErr == nil {
				if err != nil {
					t.Errorf("ValidateMetadataSize(%v, %d) = %v, want nil", tt.metadata, tt.maxSize, err)
				}
			} else {
				if err == nil {
					t.Errorf("ValidateMetadataSize(%v, %d) = nil, want error wrapping %v", tt.metadata, tt.maxSize, tt.wantErr)
				} else if !errors.Is(err, tt.wantErr) {
					t.Errorf("ValidateMetadataSize(%v, %d) = %v, want error wrapping %v", tt.metadata, tt.maxSize, err, tt.wantErr)
				}
			}
		})
	}
}

func TestIsValidStreamNameChar(t *testing.T) {
	validChars := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789-_"
	invalidChars := " !@#$%^&*()+=[]{}|\\:;\"'<>,.?/~`"

	for _, r := range validChars {
		if !isValidStreamNameChar(r) {
			t.Errorf("isValidStreamNameChar(%q) = false, want true", r)
		}
	}

	for _, r := range invalidChars {
		if isValidStreamNameChar(r) {
			t.Errorf("isValidStreamNameChar(%q) = true, want false", r)
		}
	}
}
