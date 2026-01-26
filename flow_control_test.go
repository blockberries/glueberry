package glueberry

import (
	"testing"

	"github.com/multiformats/go-multiaddr"
)

func TestConfig_FlowControl_Defaults(t *testing.T) {
	cfg := &Config{}
	cfg.applyDefaults()

	if cfg.HighWatermark != DefaultHighWatermark {
		t.Errorf("expected high watermark %d, got %d", DefaultHighWatermark, cfg.HighWatermark)
	}
	if cfg.LowWatermark != DefaultLowWatermark {
		t.Errorf("expected low watermark %d, got %d", DefaultLowWatermark, cfg.LowWatermark)
	}
	if cfg.MaxMessageSize != DefaultMaxMessageSize {
		t.Errorf("expected max message size %d, got %d", DefaultMaxMessageSize, cfg.MaxMessageSize)
	}
	if cfg.DisableBackpressure {
		t.Error("backpressure should be enabled by default")
	}
}

func TestConfig_FlowControl_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     *Config
		wantErr bool
	}{
		{
			name: "negative high watermark",
			cfg: &Config{
				HighWatermark: -1,
			},
			wantErr: true,
		},
		{
			name: "negative low watermark",
			cfg: &Config{
				LowWatermark: -1,
			},
			wantErr: true,
		},
		{
			name: "low watermark >= high watermark",
			cfg: &Config{
				HighWatermark: 100,
				LowWatermark:  100,
			},
			wantErr: true,
		},
		{
			name: "low watermark > high watermark",
			cfg: &Config{
				HighWatermark: 100,
				LowWatermark:  200,
			},
			wantErr: true,
		},
		{
			name: "negative max message size",
			cfg: &Config{
				MaxMessageSize: -1,
			},
			wantErr: true,
		},
		{
			name: "valid flow control config",
			cfg: &Config{
				HighWatermark:  1000,
				LowWatermark:   100,
				MaxMessageSize: 1024 * 1024,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Add required fields
			tt.cfg.PrivateKey = make([]byte, 64)
			tt.cfg.AddressBookPath = "/tmp/test"
			ma, _ := multiaddr.NewMultiaddr("/ip4/127.0.0.1/tcp/0")
			tt.cfg.ListenAddrs = []multiaddr.Multiaddr{ma}

			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestWithHighWatermark(t *testing.T) {
	cfg := &Config{}
	opt := WithHighWatermark(500)
	opt(cfg)

	if cfg.HighWatermark != 500 {
		t.Errorf("expected high watermark 500, got %d", cfg.HighWatermark)
	}
}

func TestWithLowWatermark(t *testing.T) {
	cfg := &Config{}
	opt := WithLowWatermark(50)
	opt(cfg)

	if cfg.LowWatermark != 50 {
		t.Errorf("expected low watermark 50, got %d", cfg.LowWatermark)
	}
}

func TestWithMaxMessageSize(t *testing.T) {
	cfg := &Config{}
	opt := WithMaxMessageSize(1024)
	opt(cfg)

	if cfg.MaxMessageSize != 1024 {
		t.Errorf("expected max message size 1024, got %d", cfg.MaxMessageSize)
	}
}

func TestWithBackpressureDisabled(t *testing.T) {
	cfg := &Config{}
	opt := WithBackpressureDisabled()
	opt(cfg)

	if !cfg.DisableBackpressure {
		t.Error("expected backpressure to be disabled")
	}
}

func TestErrCodeMessageTooLarge_String(t *testing.T) {
	code := ErrCodeMessageTooLarge
	str := code.String()

	if str != "MessageTooLarge" {
		t.Errorf("expected %q, got %q", "MessageTooLarge", str)
	}
}

func TestErrCodeBackpressure_String(t *testing.T) {
	code := ErrCodeBackpressure
	str := code.String()

	if str != "Backpressure" {
		t.Errorf("expected %q, got %q", "Backpressure", str)
	}
}
