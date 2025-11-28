package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestArliaiClient_CheckConnection(t *testing.T) {
	// Мок сервер для тестирования
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"ok","model":"GLM-4.5-Air","version":"1.0"}`))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := &ArliaiClient{
		baseURL:    server.URL,
		apiKey:     "test-key",
		httpClient: &http.Client{Timeout: 5 * time.Second},
		retryConfig: RetryConfig{
			MaxRetries:       2,
			InitialDelay:     100 * time.Millisecond,
			MaxDelay:         1 * time.Second,
			BackoffMultiplier: 2.0,
		},
	}

	ctx := context.Background()
	status, err := client.CheckConnection(ctx, "test-request-id")

	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if status.Status != "ok" {
		t.Errorf("Expected status 'ok', got: %s", status.Status)
	}

	if status.Model != "GLM-4.5-Air" {
		t.Errorf("Expected model 'GLM-4.5-Air', got: %s", status.Model)
	}
}

func TestArliaiClient_CheckConnection_Retry(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 2 {
			w.WriteHeader(http.StatusInternalServerError)
		} else {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status":"ok"}`))
		}
	}))
	defer server.Close()

	client := &ArliaiClient{
		baseURL:    server.URL,
		apiKey:     "test-key",
		httpClient: &http.Client{Timeout: 5 * time.Second},
		retryConfig: RetryConfig{
			MaxRetries:       2,
			InitialDelay:     50 * time.Millisecond,
			MaxDelay:         500 * time.Millisecond,
			BackoffMultiplier: 2.0,
		},
	}

	ctx := context.Background()
	status, err := client.CheckConnection(ctx, "test-retry-id")

	if err != nil {
		t.Fatalf("Expected no error after retry, got: %v", err)
	}

	if status.Status != "ok" {
		t.Errorf("Expected status 'ok', got: %s", status.Status)
	}

	if attempts != 2 {
		t.Errorf("Expected 2 attempts, got: %d", attempts)
	}
}

func TestArliaiCache(t *testing.T) {
	cache := NewArliaiCache()

	// Тест SetStatus и GetStatus
	testData := map[string]interface{}{
		"connected": true,
		"model":     "GLM-4.5-Air",
	}

	cache.SetStatus(testData)

	cached, ok := cache.GetStatus()
	if !ok {
		t.Fatal("Expected cached status to be available")
	}

	cachedMap, ok := cached.(map[string]interface{})
	if !ok {
		t.Fatal("Expected cached data to be map")
	}

	if cachedMap["connected"] != true {
		t.Errorf("Expected connected=true, got: %v", cachedMap["connected"])
	}

	// Тест истечения TTL
	time.Sleep(100 * time.Millisecond)
	// Уменьшаем TTL для теста
	cache.statusTTL = 50 * time.Millisecond
	cache.SetStatus(testData)
	time.Sleep(100 * time.Millisecond)

	_, ok = cache.GetStatus()
	if ok {
		t.Error("Expected cache to be expired")
	}
}

func TestGenerateTraceID(t *testing.T) {
	id1 := GenerateTraceID()
	id2 := GenerateTraceID()

	if id1 == id2 {
		t.Error("Expected different trace IDs")
	}

	if len(id1) == 0 {
		t.Error("Expected non-empty trace ID")
	}
}

