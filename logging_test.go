package glueberry

import (
	"sync"
	"testing"
)

func TestNopLogger_Implements_Logger(t *testing.T) {
	var _ Logger = NopLogger{}
}

func TestNopLogger_Methods_DoNotPanic(t *testing.T) {
	logger := NopLogger{}

	// Should not panic with any arguments
	logger.Debug("message")
	logger.Debug("message", "key", "value")
	logger.Debug("message", "key1", "value1", "key2", "value2")
	logger.Info("message")
	logger.Info("message", "key", 123)
	logger.Warn("message")
	logger.Warn("message", "key", struct{}{})
	logger.Error("message")
	logger.Error("message", "key", nil)
}

// TestLogger is a test logger that records log calls.
type TestLogger struct {
	mu     sync.Mutex
	Calls  []LogCall
	Levels []string
}

type LogCall struct {
	Level         string
	Message       string
	KeysAndValues []any
}

func (l *TestLogger) Debug(msg string, keysAndValues ...any) {
	l.record("debug", msg, keysAndValues)
}

func (l *TestLogger) Info(msg string, keysAndValues ...any) {
	l.record("info", msg, keysAndValues)
}

func (l *TestLogger) Warn(msg string, keysAndValues ...any) {
	l.record("warn", msg, keysAndValues)
}

func (l *TestLogger) Error(msg string, keysAndValues ...any) {
	l.record("error", msg, keysAndValues)
}

func (l *TestLogger) record(level, msg string, keysAndValues []any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.Calls = append(l.Calls, LogCall{
		Level:         level,
		Message:       msg,
		KeysAndValues: keysAndValues,
	})
	l.Levels = append(l.Levels, level)
}

func TestTestLogger_RecordsCalls(t *testing.T) {
	logger := &TestLogger{}

	logger.Debug("debug message", "key1", "value1")
	logger.Info("info message", "key2", 123)
	logger.Warn("warn message")
	logger.Error("error message", "err", "some error")

	if len(logger.Calls) != 4 {
		t.Errorf("expected 4 calls, got %d", len(logger.Calls))
	}

	// Check debug call
	if logger.Calls[0].Level != "debug" || logger.Calls[0].Message != "debug message" {
		t.Errorf("unexpected debug call: %+v", logger.Calls[0])
	}

	// Check info call
	if logger.Calls[1].Level != "info" || logger.Calls[1].Message != "info message" {
		t.Errorf("unexpected info call: %+v", logger.Calls[1])
	}

	// Check warn call
	if logger.Calls[2].Level != "warn" || logger.Calls[2].Message != "warn message" {
		t.Errorf("unexpected warn call: %+v", logger.Calls[2])
	}

	// Check error call
	if logger.Calls[3].Level != "error" || logger.Calls[3].Message != "error message" {
		t.Errorf("unexpected error call: %+v", logger.Calls[3])
	}
}

func TestLogger_IsThreadSafe(t *testing.T) {
	logger := &TestLogger{}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(4)
		go func() {
			defer wg.Done()
			logger.Debug("debug")
		}()
		go func() {
			defer wg.Done()
			logger.Info("info")
		}()
		go func() {
			defer wg.Done()
			logger.Warn("warn")
		}()
		go func() {
			defer wg.Done()
			logger.Error("error")
		}()
	}
	wg.Wait()

	// Should have recorded 400 calls
	if len(logger.Calls) != 400 {
		t.Errorf("expected 400 calls, got %d", len(logger.Calls))
	}
}

func TestWithLogger_SetsLogger(t *testing.T) {
	testLogger := &TestLogger{}

	cfg := &Config{}
	opt := WithLogger(testLogger)
	opt(cfg)

	if cfg.Logger != testLogger {
		t.Error("WithLogger should set the logger")
	}
}

func TestConfig_DefaultsToNopLogger(t *testing.T) {
	cfg := &Config{}
	cfg.applyDefaults()

	if cfg.Logger == nil {
		t.Error("applyDefaults should set NopLogger")
	}

	// Verify it's a NopLogger by type assertion
	_, ok := cfg.Logger.(NopLogger)
	if !ok {
		t.Error("default logger should be NopLogger")
	}
}

func TestConfig_WithLogger_OverridesDefault(t *testing.T) {
	testLogger := &TestLogger{}

	cfg := &Config{Logger: testLogger}
	cfg.applyDefaults()

	// Should not override when already set
	if cfg.Logger != testLogger {
		t.Error("applyDefaults should not override existing logger")
	}
}
