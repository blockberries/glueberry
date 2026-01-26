package glueberry

import "testing"

func TestProtocolVersion_String(t *testing.T) {
	tests := []struct {
		v    ProtocolVersion
		want string
	}{
		{ProtocolVersion{1, 0, 0}, "1.0.0"},
		{ProtocolVersion{1, 2, 3}, "1.2.3"},
		{ProtocolVersion{0, 0, 0}, "0.0.0"},
		{ProtocolVersion{255, 255, 255}, "255.255.255"},
	}

	for _, tt := range tests {
		got := tt.v.String()
		if got != tt.want {
			t.Errorf("ProtocolVersion%v.String() = %q, want %q", tt.v, got, tt.want)
		}
	}
}

func TestProtocolVersion_Compatible(t *testing.T) {
	tests := []struct {
		name   string
		v      ProtocolVersion
		other  ProtocolVersion
		compat bool
	}{
		// Same versions are compatible
		{"same version", ProtocolVersion{1, 0, 0}, ProtocolVersion{1, 0, 0}, true},
		{"same with patch diff", ProtocolVersion{1, 0, 5}, ProtocolVersion{1, 0, 0}, true},

		// Older minor versions are compatible
		{"older minor", ProtocolVersion{1, 2, 0}, ProtocolVersion{1, 0, 0}, true},
		{"older minor with patch", ProtocolVersion{1, 2, 3}, ProtocolVersion{1, 1, 5}, true},

		// Newer minor versions are NOT compatible (peer has features we don't support)
		{"newer minor", ProtocolVersion{1, 0, 0}, ProtocolVersion{1, 2, 0}, false},
		{"newer minor with patch", ProtocolVersion{1, 1, 0}, ProtocolVersion{1, 2, 0}, false},

		// Different major versions are NOT compatible
		{"different major up", ProtocolVersion{1, 0, 0}, ProtocolVersion{2, 0, 0}, false},
		{"different major down", ProtocolVersion{2, 0, 0}, ProtocolVersion{1, 0, 0}, false},
		{"major 0 to 1", ProtocolVersion{1, 0, 0}, ProtocolVersion{0, 9, 0}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.v.Compatible(tt.other)
			if got != tt.compat {
				t.Errorf("%v.Compatible(%v) = %v, want %v", tt.v, tt.other, got, tt.compat)
			}
		})
	}
}

func TestProtocolVersion_IsNewer(t *testing.T) {
	tests := []struct {
		name  string
		v     ProtocolVersion
		other ProtocolVersion
		newer bool
	}{
		{"same", ProtocolVersion{1, 0, 0}, ProtocolVersion{1, 0, 0}, false},
		{"major newer", ProtocolVersion{2, 0, 0}, ProtocolVersion{1, 9, 9}, true},
		{"major older", ProtocolVersion{1, 9, 9}, ProtocolVersion{2, 0, 0}, false},
		{"minor newer", ProtocolVersion{1, 2, 0}, ProtocolVersion{1, 1, 9}, true},
		{"minor older", ProtocolVersion{1, 1, 9}, ProtocolVersion{1, 2, 0}, false},
		{"patch newer", ProtocolVersion{1, 0, 5}, ProtocolVersion{1, 0, 4}, true},
		{"patch older", ProtocolVersion{1, 0, 4}, ProtocolVersion{1, 0, 5}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.v.IsNewer(tt.other)
			if got != tt.newer {
				t.Errorf("%v.IsNewer(%v) = %v, want %v", tt.v, tt.other, got, tt.newer)
			}
		})
	}
}

func TestProtocolVersion_Equal(t *testing.T) {
	tests := []struct {
		v     ProtocolVersion
		other ProtocolVersion
		equal bool
	}{
		{ProtocolVersion{1, 0, 0}, ProtocolVersion{1, 0, 0}, true},
		{ProtocolVersion{1, 0, 0}, ProtocolVersion{1, 0, 1}, false},
		{ProtocolVersion{1, 0, 0}, ProtocolVersion{1, 1, 0}, false},
		{ProtocolVersion{1, 0, 0}, ProtocolVersion{2, 0, 0}, false},
	}

	for _, tt := range tests {
		got := tt.v.Equal(tt.other)
		if got != tt.equal {
			t.Errorf("%v.Equal(%v) = %v, want %v", tt.v, tt.other, got, tt.equal)
		}
	}
}

func TestParseVersion(t *testing.T) {
	tests := []struct {
		s       string
		want    ProtocolVersion
		wantErr bool
	}{
		{"1.0.0", ProtocolVersion{1, 0, 0}, false},
		{"1.2.3", ProtocolVersion{1, 2, 3}, false},
		{"255.255.255", ProtocolVersion{255, 255, 255}, false},
		{"0.0.0", ProtocolVersion{0, 0, 0}, false},
		{"invalid", ProtocolVersion{}, true},
		{"1.0", ProtocolVersion{}, true},
		{"1", ProtocolVersion{}, true},
		{"", ProtocolVersion{}, true},
		{"1.0.0.0", ProtocolVersion{1, 0, 0}, false}, // Parses first 3
	}

	for _, tt := range tests {
		t.Run(tt.s, func(t *testing.T) {
			got, err := ParseVersion(tt.s)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseVersion(%q) error = %v, wantErr %v", tt.s, err, tt.wantErr)
				return
			}
			if !tt.wantErr && !got.Equal(tt.want) {
				t.Errorf("ParseVersion(%q) = %v, want %v", tt.s, got, tt.want)
			}
		})
	}
}

func TestCurrentVersion(t *testing.T) {
	v := CurrentVersion()

	if v.Major != ProtocolVersionMajor {
		t.Errorf("CurrentVersion().Major = %d, want %d", v.Major, ProtocolVersionMajor)
	}
	if v.Minor != ProtocolVersionMinor {
		t.Errorf("CurrentVersion().Minor = %d, want %d", v.Minor, ProtocolVersionMinor)
	}
	if v.Patch != ProtocolVersionPatch {
		t.Errorf("CurrentVersion().Patch = %d, want %d", v.Patch, ProtocolVersionPatch)
	}

	// Should be self-compatible
	if !v.Compatible(v) {
		t.Error("CurrentVersion should be compatible with itself")
	}
}

func TestErrCodeVersionMismatch(t *testing.T) {
	code := ErrCodeVersionMismatch
	str := code.String()

	if str != "VersionMismatch" {
		t.Errorf("ErrCodeVersionMismatch.String() = %q, want %q", str, "VersionMismatch")
	}
}
