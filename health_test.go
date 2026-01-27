package glueberry

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNode_IsHealthy_NotStarted(t *testing.T) {
	// A node that hasn't been started should not be healthy
	node := &Node{}
	if node.IsHealthy() {
		t.Error("expected unstarted node to be unhealthy")
	}
}

func TestNode_ReadinessChecks_NotStarted(t *testing.T) {
	node := &Node{}
	status := node.ReadinessChecks()

	if status.Healthy {
		t.Error("expected unstarted node health status to be unhealthy")
	}

	// Check that we have the expected checks
	if len(status.Checks) != 4 {
		t.Errorf("expected 4 checks, got %d", len(status.Checks))
	}

	// First check should be node_started and unhealthy
	found := false
	for _, check := range status.Checks {
		if check.Name == "node_started" {
			found = true
			if check.Healthy {
				t.Error("expected node_started check to be unhealthy")
			}
		}
	}
	if !found {
		t.Error("expected node_started check to be present")
	}
}

func TestHealthHandler_Unhealthy(t *testing.T) {
	node := &Node{}
	handler := HealthHandler(node)

	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", rec.Code)
	}

	var status HealthStatus
	if err := json.NewDecoder(rec.Body).Decode(&status); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if status.Healthy {
		t.Error("expected unhealthy status in response")
	}
}

func TestLivenessHandler_Unhealthy(t *testing.T) {
	node := &Node{}
	handler := LivenessHandler(node)

	req := httptest.NewRequest("GET", "/live", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", rec.Code)
	}

	var result struct {
		Healthy bool `json:"healthy"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result.Healthy {
		t.Error("expected unhealthy status in response")
	}
}

func TestCheckResult_Fields(t *testing.T) {
	result := CheckResult{
		Name:    "test_check",
		Healthy: true,
		Message: "everything is fine",
	}

	if result.Name != "test_check" {
		t.Errorf("Name = %q, want %q", result.Name, "test_check")
	}
	if !result.Healthy {
		t.Error("expected Healthy to be true")
	}
	if result.Message != "everything is fine" {
		t.Errorf("Message = %q, want %q", result.Message, "everything is fine")
	}
}

func TestHealthStatus_Fields(t *testing.T) {
	status := HealthStatus{
		Healthy: true,
		Checks: []CheckResult{
			{Name: "check1", Healthy: true},
			{Name: "check2", Healthy: true},
		},
	}

	if !status.Healthy {
		t.Error("expected Healthy to be true")
	}
	if len(status.Checks) != 2 {
		t.Errorf("expected 2 checks, got %d", len(status.Checks))
	}
}

func TestBoolToMessage(t *testing.T) {
	tests := []struct {
		b       bool
		trueMsg string
		falseMsg string
		want    string
	}{
		{true, "yes", "no", "yes"},
		{false, "yes", "no", "no"},
		{true, "", "empty", ""},
		{false, "yes", "", ""},
	}

	for _, tt := range tests {
		got := boolToMessage(tt.b, tt.trueMsg, tt.falseMsg)
		if got != tt.want {
			t.Errorf("boolToMessage(%v, %q, %q) = %q, want %q",
				tt.b, tt.trueMsg, tt.falseMsg, got, tt.want)
		}
	}
}

func TestHealthHandler_ContentType(t *testing.T) {
	node := &Node{}
	handler := HealthHandler(node)

	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %q, want %q", contentType, "application/json")
	}
}

func TestLivenessHandler_ContentType(t *testing.T) {
	node := &Node{}
	handler := LivenessHandler(node)

	req := httptest.NewRequest("GET", "/live", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	contentType := rec.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %q, want %q", contentType, "application/json")
	}
}
