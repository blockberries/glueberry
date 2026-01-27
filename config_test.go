package glueberry

import (
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
)

func generateTestKey(t *testing.T) ed25519.PrivateKey {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	return priv
}

func mustParseMultiaddr(t *testing.T, s string) multiaddr.Multiaddr {
	t.Helper()
	ma, err := multiaddr.NewMultiaddr(s)
	if err != nil {
		t.Fatalf("failed to parse multiaddr %q: %v", s, err)
	}
	return ma
}

func TestConfig_Validate_RequiredFields(t *testing.T) {
	validKey := generateTestKey(t)
	validAddr := mustParseMultiaddr(t, "/ip4/127.0.0.1/tcp/9000")

	tests := []struct {
		name      string
		config    Config
		wantErr   error
		wantNoErr bool
	}{
		{
			name:    "missing private key",
			config:  Config{AddressBookPath: "/tmp/ab.json", ListenAddrs: []multiaddr.Multiaddr{validAddr}},
			wantErr: ErrMissingPrivateKey,
		},
		{
			name:    "nil private key",
			config:  Config{PrivateKey: nil, AddressBookPath: "/tmp/ab.json", ListenAddrs: []multiaddr.Multiaddr{validAddr}},
			wantErr: ErrMissingPrivateKey,
		},
		{
			name:    "invalid private key size",
			config:  Config{PrivateKey: []byte("short"), AddressBookPath: "/tmp/ab.json", ListenAddrs: []multiaddr.Multiaddr{validAddr}},
			wantErr: ErrInvalidPrivateKey,
		},
		{
			name:    "missing address book path",
			config:  Config{PrivateKey: validKey, AddressBookPath: "", ListenAddrs: []multiaddr.Multiaddr{validAddr}},
			wantErr: ErrMissingAddressBookPath,
		},
		{
			name:    "missing listen addrs",
			config:  Config{PrivateKey: validKey, AddressBookPath: "/tmp/ab.json", ListenAddrs: nil},
			wantErr: ErrMissingListenAddrs,
		},
		{
			name:    "empty listen addrs",
			config:  Config{PrivateKey: validKey, AddressBookPath: "/tmp/ab.json", ListenAddrs: []multiaddr.Multiaddr{}},
			wantErr: ErrMissingListenAddrs,
		},
		{
			name:      "valid minimal config",
			config:    Config{PrivateKey: validKey, AddressBookPath: "/tmp/ab.json", ListenAddrs: []multiaddr.Multiaddr{validAddr}},
			wantNoErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantNoErr {
				if err != nil {
					t.Errorf("expected no error, got %v", err)
				}
				return
			}
			if err == nil {
				t.Error("expected error, got nil")
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("expected error %v, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestConfig_Validate_OptionalFields(t *testing.T) {
	validKey := generateTestKey(t)
	validAddr := mustParseMultiaddr(t, "/ip4/127.0.0.1/tcp/9000")

	baseConfig := func() Config {
		return Config{
			PrivateKey:      validKey,
			AddressBookPath: "/tmp/ab.json",
			ListenAddrs:     []multiaddr.Multiaddr{validAddr},
		}
	}

	tests := []struct {
		name    string
		modify  func(*Config)
		wantErr bool
	}{
		{
			name:    "negative handshake timeout",
			modify:  func(c *Config) { c.HandshakeTimeout = -1 * time.Second },
			wantErr: true,
		},
		{
			name:    "negative reconnect base delay",
			modify:  func(c *Config) { c.ReconnectBaseDelay = -1 * time.Second },
			wantErr: true,
		},
		{
			name:    "negative reconnect max delay",
			modify:  func(c *Config) { c.ReconnectMaxDelay = -1 * time.Second },
			wantErr: true,
		},
		{
			name: "reconnect max delay less than base delay",
			modify: func(c *Config) {
				c.ReconnectBaseDelay = 10 * time.Second
				c.ReconnectMaxDelay = 5 * time.Second
			},
			wantErr: true,
		},
		{
			name:    "negative reconnect max attempts",
			modify:  func(c *Config) { c.ReconnectMaxAttempts = -1 },
			wantErr: true,
		},
		{
			name:    "zero reconnect max attempts is valid (unlimited)",
			modify:  func(c *Config) { c.ReconnectMaxAttempts = 0 },
			wantErr: false,
		},
		{
			name:    "negative failed handshake cooldown",
			modify:  func(c *Config) { c.FailedHandshakeCooldown = -1 * time.Second },
			wantErr: true,
		},
		{
			name:    "negative event buffer size",
			modify:  func(c *Config) { c.EventBufferSize = -1 },
			wantErr: true,
		},
		{
			name:    "negative message buffer size",
			modify:  func(c *Config) { c.MessageBufferSize = -1 },
			wantErr: true,
		},
		{
			name:    "zero event buffer size is valid",
			modify:  func(c *Config) { c.EventBufferSize = 0 },
			wantErr: false,
		},
		{
			name:    "zero message buffer size is valid",
			modify:  func(c *Config) { c.MessageBufferSize = 0 },
			wantErr: false,
		},
		{
			name: "valid custom timeouts",
			modify: func(c *Config) {
				c.HandshakeTimeout = 60 * time.Second
				c.ReconnectBaseDelay = 2 * time.Second
				c.ReconnectMaxDelay = 10 * time.Minute
				c.ReconnectMaxAttempts = 20
				c.FailedHandshakeCooldown = 5 * time.Minute
			},
			wantErr: false,
		},
		{
			name:    "negative conn mgr low watermark",
			modify:  func(c *Config) { c.ConnMgrLowWatermark = -1 },
			wantErr: true,
		},
		{
			name:    "negative conn mgr high watermark",
			modify:  func(c *Config) { c.ConnMgrHighWatermark = -1 },
			wantErr: true,
		},
		{
			name: "conn mgr low watermark greater than high watermark",
			modify: func(c *Config) {
				c.ConnMgrLowWatermark = 500
				c.ConnMgrHighWatermark = 100
			},
			wantErr: true,
		},
		{
			name: "valid conn mgr watermarks",
			modify: func(c *Config) {
				c.ConnMgrLowWatermark = 50
				c.ConnMgrHighWatermark = 200
			},
			wantErr: false,
		},
		{
			name:    "negative max stream name length",
			modify:  func(c *Config) { c.MaxStreamNameLength = -1 },
			wantErr: true,
		},
		{
			name:    "zero max stream name length is valid (uses default)",
			modify:  func(c *Config) { c.MaxStreamNameLength = 0 },
			wantErr: false,
		},
		{
			name:    "positive max stream name length is valid",
			modify:  func(c *Config) { c.MaxStreamNameLength = 128 },
			wantErr: false,
		},
		{
			name:    "negative max metadata size",
			modify:  func(c *Config) { c.MaxMetadataSize = -1 },
			wantErr: true,
		},
		{
			name:    "zero max metadata size is valid (uses default)",
			modify:  func(c *Config) { c.MaxMetadataSize = 0 },
			wantErr: false,
		},
		{
			name:    "positive max metadata size is valid",
			modify:  func(c *Config) { c.MaxMetadataSize = 8192 },
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := baseConfig()
			tt.modify(&cfg)
			err := cfg.Validate()
			if tt.wantErr && err == nil {
				t.Error("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("expected no error, got %v", err)
			}
		})
	}
}

func TestConfig_ApplyDefaults(t *testing.T) {
	validKey := generateTestKey(t)
	validAddr := mustParseMultiaddr(t, "/ip4/127.0.0.1/tcp/9000")

	cfg := &Config{
		PrivateKey:      validKey,
		AddressBookPath: "/tmp/ab.json",
		ListenAddrs:     []multiaddr.Multiaddr{validAddr},
	}

	cfg.applyDefaults()

	if cfg.HandshakeTimeout != DefaultHandshakeTimeout {
		t.Errorf("HandshakeTimeout = %v, want %v", cfg.HandshakeTimeout, DefaultHandshakeTimeout)
	}
	if cfg.ReconnectBaseDelay != DefaultReconnectBaseDelay {
		t.Errorf("ReconnectBaseDelay = %v, want %v", cfg.ReconnectBaseDelay, DefaultReconnectBaseDelay)
	}
	if cfg.ReconnectMaxDelay != DefaultReconnectMaxDelay {
		t.Errorf("ReconnectMaxDelay = %v, want %v", cfg.ReconnectMaxDelay, DefaultReconnectMaxDelay)
	}
	if cfg.ReconnectMaxAttempts != DefaultReconnectMaxAttempts {
		t.Errorf("ReconnectMaxAttempts = %v, want %v", cfg.ReconnectMaxAttempts, DefaultReconnectMaxAttempts)
	}
	if cfg.FailedHandshakeCooldown != DefaultFailedHandshakeCooldown {
		t.Errorf("FailedHandshakeCooldown = %v, want %v", cfg.FailedHandshakeCooldown, DefaultFailedHandshakeCooldown)
	}
	if cfg.EventBufferSize != DefaultEventBufferSize {
		t.Errorf("EventBufferSize = %v, want %v", cfg.EventBufferSize, DefaultEventBufferSize)
	}
	if cfg.MessageBufferSize != DefaultMessageBufferSize {
		t.Errorf("MessageBufferSize = %v, want %v", cfg.MessageBufferSize, DefaultMessageBufferSize)
	}
	if cfg.ConnMgrLowWatermark != DefaultConnMgrLowWatermark {
		t.Errorf("ConnMgrLowWatermark = %v, want %v", cfg.ConnMgrLowWatermark, DefaultConnMgrLowWatermark)
	}
	if cfg.ConnMgrHighWatermark != DefaultConnMgrHighWatermark {
		t.Errorf("ConnMgrHighWatermark = %v, want %v", cfg.ConnMgrHighWatermark, DefaultConnMgrHighWatermark)
	}
	if cfg.MaxStreamNameLength != DefaultMaxStreamNameLength {
		t.Errorf("MaxStreamNameLength = %v, want %v", cfg.MaxStreamNameLength, DefaultMaxStreamNameLength)
	}
	if cfg.MaxMetadataSize != DefaultMaxMetadataSize {
		t.Errorf("MaxMetadataSize = %v, want %v", cfg.MaxMetadataSize, DefaultMaxMetadataSize)
	}
}

func TestConfig_ApplyDefaults_DoesNotOverrideSet(t *testing.T) {
	validKey := generateTestKey(t)
	validAddr := mustParseMultiaddr(t, "/ip4/127.0.0.1/tcp/9000")

	customTimeout := 45 * time.Second
	customBaseDelay := 3 * time.Second
	customMaxDelay := 2 * time.Minute
	customMaxAttempts := 5
	customCooldown := 30 * time.Second
	customEventBuffer := 50
	customMessageBuffer := 500

	cfg := &Config{
		PrivateKey:              validKey,
		AddressBookPath:         "/tmp/ab.json",
		ListenAddrs:             []multiaddr.Multiaddr{validAddr},
		HandshakeTimeout:        customTimeout,
		ReconnectBaseDelay:      customBaseDelay,
		ReconnectMaxDelay:       customMaxDelay,
		ReconnectMaxAttempts:    customMaxAttempts,
		FailedHandshakeCooldown: customCooldown,
		EventBufferSize:         customEventBuffer,
		MessageBufferSize:       customMessageBuffer,
	}

	cfg.applyDefaults()

	if cfg.HandshakeTimeout != customTimeout {
		t.Errorf("HandshakeTimeout = %v, want %v", cfg.HandshakeTimeout, customTimeout)
	}
	if cfg.ReconnectBaseDelay != customBaseDelay {
		t.Errorf("ReconnectBaseDelay = %v, want %v", cfg.ReconnectBaseDelay, customBaseDelay)
	}
	if cfg.ReconnectMaxDelay != customMaxDelay {
		t.Errorf("ReconnectMaxDelay = %v, want %v", cfg.ReconnectMaxDelay, customMaxDelay)
	}
	if cfg.ReconnectMaxAttempts != customMaxAttempts {
		t.Errorf("ReconnectMaxAttempts = %v, want %v", cfg.ReconnectMaxAttempts, customMaxAttempts)
	}
	if cfg.FailedHandshakeCooldown != customCooldown {
		t.Errorf("FailedHandshakeCooldown = %v, want %v", cfg.FailedHandshakeCooldown, customCooldown)
	}
	if cfg.EventBufferSize != customEventBuffer {
		t.Errorf("EventBufferSize = %v, want %v", cfg.EventBufferSize, customEventBuffer)
	}
	if cfg.MessageBufferSize != customMessageBuffer {
		t.Errorf("MessageBufferSize = %v, want %v", cfg.MessageBufferSize, customMessageBuffer)
	}
}

func TestNewConfig(t *testing.T) {
	validKey := generateTestKey(t)
	validAddr := mustParseMultiaddr(t, "/ip4/127.0.0.1/tcp/9000")

	cfg := NewConfig(validKey, "/tmp/ab.json", []multiaddr.Multiaddr{validAddr})

	if cfg.PrivateKey == nil {
		t.Error("PrivateKey should not be nil")
	}
	if cfg.AddressBookPath != "/tmp/ab.json" {
		t.Errorf("AddressBookPath = %q, want %q", cfg.AddressBookPath, "/tmp/ab.json")
	}
	if len(cfg.ListenAddrs) != 1 {
		t.Errorf("ListenAddrs length = %d, want 1", len(cfg.ListenAddrs))
	}
	// Defaults should be applied
	if cfg.HandshakeTimeout != DefaultHandshakeTimeout {
		t.Errorf("HandshakeTimeout = %v, want %v", cfg.HandshakeTimeout, DefaultHandshakeTimeout)
	}
}

func TestNewConfig_WithOptions(t *testing.T) {
	validKey := generateTestKey(t)
	validAddr := mustParseMultiaddr(t, "/ip4/127.0.0.1/tcp/9000")

	customTimeout := 90 * time.Second
	customBaseDelay := 5 * time.Second
	customMaxDelay := 15 * time.Minute
	customMaxAttempts := 3
	customCooldown := 2 * time.Minute
	customEventBuffer := 200
	customMessageBuffer := 2000

	cfg := NewConfig(
		validKey,
		"/tmp/ab.json",
		[]multiaddr.Multiaddr{validAddr},
		WithHandshakeTimeout(customTimeout),
		WithReconnectBaseDelay(customBaseDelay),
		WithReconnectMaxDelay(customMaxDelay),
		WithReconnectMaxAttempts(customMaxAttempts),
		WithFailedHandshakeCooldown(customCooldown),
		WithEventBufferSize(customEventBuffer),
		WithMessageBufferSize(customMessageBuffer),
	)

	if cfg.HandshakeTimeout != customTimeout {
		t.Errorf("HandshakeTimeout = %v, want %v", cfg.HandshakeTimeout, customTimeout)
	}
	if cfg.ReconnectBaseDelay != customBaseDelay {
		t.Errorf("ReconnectBaseDelay = %v, want %v", cfg.ReconnectBaseDelay, customBaseDelay)
	}
	if cfg.ReconnectMaxDelay != customMaxDelay {
		t.Errorf("ReconnectMaxDelay = %v, want %v", cfg.ReconnectMaxDelay, customMaxDelay)
	}
	if cfg.ReconnectMaxAttempts != customMaxAttempts {
		t.Errorf("ReconnectMaxAttempts = %v, want %v", cfg.ReconnectMaxAttempts, customMaxAttempts)
	}
	if cfg.FailedHandshakeCooldown != customCooldown {
		t.Errorf("FailedHandshakeCooldown = %v, want %v", cfg.FailedHandshakeCooldown, customCooldown)
	}
	if cfg.EventBufferSize != customEventBuffer {
		t.Errorf("EventBufferSize = %v, want %v", cfg.EventBufferSize, customEventBuffer)
	}
	if cfg.MessageBufferSize != customMessageBuffer {
		t.Errorf("MessageBufferSize = %v, want %v", cfg.MessageBufferSize, customMessageBuffer)
	}
}

func TestConfigOptions_Individual(t *testing.T) {
	tests := []struct {
		name   string
		option ConfigOption
		check  func(*Config) bool
	}{
		{
			name:   "WithHandshakeTimeout",
			option: WithHandshakeTimeout(45 * time.Second),
			check:  func(c *Config) bool { return c.HandshakeTimeout == 45*time.Second },
		},
		{
			name:   "WithReconnectBaseDelay",
			option: WithReconnectBaseDelay(3 * time.Second),
			check:  func(c *Config) bool { return c.ReconnectBaseDelay == 3*time.Second },
		},
		{
			name:   "WithReconnectMaxDelay",
			option: WithReconnectMaxDelay(8 * time.Minute),
			check:  func(c *Config) bool { return c.ReconnectMaxDelay == 8*time.Minute },
		},
		{
			name:   "WithReconnectMaxAttempts",
			option: WithReconnectMaxAttempts(7),
			check:  func(c *Config) bool { return c.ReconnectMaxAttempts == 7 },
		},
		{
			name:   "WithFailedHandshakeCooldown",
			option: WithFailedHandshakeCooldown(90 * time.Second),
			check:  func(c *Config) bool { return c.FailedHandshakeCooldown == 90*time.Second },
		},
		{
			name:   "WithEventBufferSize",
			option: WithEventBufferSize(150),
			check:  func(c *Config) bool { return c.EventBufferSize == 150 },
		},
		{
			name:   "WithMessageBufferSize",
			option: WithMessageBufferSize(1500),
			check:  func(c *Config) bool { return c.MessageBufferSize == 1500 },
		},
		{
			name:   "WithConnMgrLowWatermark",
			option: WithConnMgrLowWatermark(75),
			check:  func(c *Config) bool { return c.ConnMgrLowWatermark == 75 },
		},
		{
			name:   "WithConnMgrHighWatermark",
			option: WithConnMgrHighWatermark(300),
			check:  func(c *Config) bool { return c.ConnMgrHighWatermark == 300 },
		},
		{
			name:   "WithDecryptionErrorCallback",
			option: WithDecryptionErrorCallback(func(peerID peer.ID, err error) {}),
			check:  func(c *Config) bool { return c.OnDecryptionError != nil },
		},
		{
			name:   "WithMaxStreamNameLength",
			option: WithMaxStreamNameLength(128),
			check:  func(c *Config) bool { return c.MaxStreamNameLength == 128 },
		},
		{
			name:   "WithMaxMetadataSize",
			option: WithMaxMetadataSize(8192),
			check:  func(c *Config) bool { return c.MaxMetadataSize == 8192 },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{}
			tt.option(cfg)
			if !tt.check(cfg) {
				t.Errorf("option %s did not set expected value", tt.name)
			}
		})
	}
}
