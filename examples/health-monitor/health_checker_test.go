package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestHealthChecker_HTTPCheck tests HTTP health check with a mock server
func TestHealthChecker_HTTPCheck(t *testing.T) {
	// Create a test server that returns 200 OK
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	checker := NewHealthChecker()

	service := &Service{
		ID:       "svc-1",
		Type:     ServiceTypeHTTP,
		Endpoint: server.URL,
		Timeout:  5000,
	}

	check, err := checker.Check(context.Background(), service)
	if err != nil {
		t.Fatalf("health check failed: %v", err)
	}

	if check.Status != HealthStatusHealthy {
		t.Errorf("expected healthy status, got %s", check.Status)
	}

	if check.ResponseTime == nil {
		t.Error("expected response time to be set")
	}

	if check.ServiceID != "svc-1" {
		t.Errorf("expected service ID svc-1, got %s", check.ServiceID)
	}
}

// TestHealthChecker_HTTPCheckUnhealthy tests HTTP health check with unhealthy status
func TestHealthChecker_HTTPCheckUnhealthy(t *testing.T) {
	// Create a test server that returns 500 Internal Server Error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	checker := NewHealthChecker()

	service := &Service{
		ID:       "svc-1",
		Type:     ServiceTypeHTTP,
		Endpoint: server.URL,
		Timeout:  5000,
	}

	check, err := checker.Check(context.Background(), service)
	if err != nil {
		t.Fatalf("health check failed: %v", err)
	}

	if check.Status != HealthStatusUnhealthy {
		t.Errorf("expected unhealthy status, got %s", check.Status)
	}

	if check.Error == nil {
		t.Error("expected error message to be set")
	}
}

// TestHealthChecker_TCPCheck tests TCP health check
// This test uses google.com:80 and should be run in integration tests
func TestHealthChecker_TCPCheck(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping TCP test in short mode (requires external network)")
	}

	checker := NewHealthChecker()

	service := &Service{
		ID:       "svc-1",
		Type:     ServiceTypeTCP,
		Endpoint: "google.com:80",
		Timeout:  5000,
	}

	check, err := checker.Check(context.Background(), service)
	if err != nil {
		t.Fatalf("health check failed: %v", err)
	}

	if check.Status != HealthStatusHealthy {
		t.Errorf("expected healthy status, got %s", check.Status)
	}

	if check.ResponseTime == nil {
		t.Error("expected response time to be set")
	}
}
