package glueberry

import "fmt"

// Protocol version constants.
// These indicate the Glueberry protocol version for compatibility checking.
const (
	// ProtocolVersionMajor is the major protocol version.
	// Breaking changes increment this.
	ProtocolVersionMajor = 1

	// ProtocolVersionMinor is the minor protocol version.
	// New features increment this.
	ProtocolVersionMinor = 2

	// ProtocolVersionPatch is the patch protocol version.
	// Bug fixes increment this.
	ProtocolVersionPatch = 4
)

// ProtocolVersion represents the Glueberry protocol version.
// Applications should exchange versions during handshake and verify compatibility.
type ProtocolVersion struct {
	// Major version - breaking changes require matching major versions.
	Major uint8

	// Minor version - new features; backwards compatible within same major.
	Minor uint8

	// Patch version - bug fixes; always compatible within same major.minor.
	Patch uint8
}

// CurrentVersion returns the current Glueberry protocol version.
func CurrentVersion() ProtocolVersion {
	return ProtocolVersion{
		Major: ProtocolVersionMajor,
		Minor: ProtocolVersionMinor,
		Patch: ProtocolVersionPatch,
	}
}

// String returns the version as a semantic version string (e.g., "1.0.0").
func (v ProtocolVersion) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// Compatible returns true if this version is compatible with another version.
// Compatibility rules:
//   - Major versions must match (breaking changes)
//   - Minor version of peer must be <= our minor version
//     (peer can't use features we don't have)
//   - Patch versions are always compatible within same major.minor
//
// Example: version 1.2.3 is compatible with 1.0.0, 1.1.0, 1.2.0, 1.2.5
// but not with 2.0.0, 0.9.0, or 1.3.0
func (v ProtocolVersion) Compatible(other ProtocolVersion) bool {
	// Major version must match
	if v.Major != other.Major {
		return false
	}

	// Peer's minor version must not exceed ours
	// (they might use features we don't support)
	if other.Minor > v.Minor {
		return false
	}

	return true
}

// IsNewer returns true if this version is newer than the other.
func (v ProtocolVersion) IsNewer(other ProtocolVersion) bool {
	if v.Major != other.Major {
		return v.Major > other.Major
	}
	if v.Minor != other.Minor {
		return v.Minor > other.Minor
	}
	return v.Patch > other.Patch
}

// Equal returns true if the versions are exactly equal.
func (v ProtocolVersion) Equal(other ProtocolVersion) bool {
	return v.Major == other.Major && v.Minor == other.Minor && v.Patch == other.Patch
}

// ParseVersion parses a version string in the format "major.minor.patch".
// Returns an error if the format is invalid.
func ParseVersion(s string) (ProtocolVersion, error) {
	var v ProtocolVersion
	n, err := fmt.Sscanf(s, "%d.%d.%d", &v.Major, &v.Minor, &v.Patch)
	if err != nil {
		return v, fmt.Errorf("invalid version format %q: %w", s, err)
	}
	if n != 3 {
		return v, fmt.Errorf("invalid version format %q: expected major.minor.patch", s)
	}
	return v, nil
}
