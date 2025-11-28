package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"httpserver/database"
	"httpserver/server"
)

// TestQualityEndpoints тестирует эндпоинты качества данных
func TestQualityEndpoints(t *testing.T) {
	db, err := database.NewDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test DB: %v", err)
	}
	defer db.Close()

	serviceDB, err := database.NewServiceDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create service DB: %v", err)
	}
	defer serviceDB.Close()

	config := &server.Config{
		Port:                    "8080",
		DatabasePath:            ":memory:",
		NormalizedDatabasePath: ":memory:",
		ServiceDatabasePath:     ":memory:",
		LogBufferSize:           100,
		NormalizerEventsBufferSize: 100,
	}

	srv := server.NewServerWithConfig(db, db, serviceDB, ":memory:", ":memory:", config)

	// Тест GET /api/quality/stats
	t.Run("GetQualityStats", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/quality/stats", nil)
		w := httptest.NewRecorder()

		srv.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}

		var response map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Errorf("Failed to parse response: %v", err)
		}
	})

	// Тест GET /api/quality/violations
	t.Run("GetQualityViolations", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/quality/violations?limit=10&offset=0", nil)
		w := httptest.NewRecorder()

		srv.ServeHTTP(w, req)

		// Может быть 200 или 404 если нет данных
		if w.Code != http.StatusOK && w.Code != http.StatusNotFound {
			t.Errorf("Expected status 200 or 404, got %d. Body: %s", w.Code, w.Body.String())
		}
	})

	// Тест GET /api/quality/suggestions
	t.Run("GetQualitySuggestions", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/quality/suggestions?limit=10", nil)
		w := httptest.NewRecorder()

		srv.ServeHTTP(w, req)

		if w.Code != http.StatusOK && w.Code != http.StatusNotFound {
			t.Errorf("Expected status 200 or 404, got %d. Body: %s", w.Code, w.Body.String())
		}
	})

	// Тест GET /api/quality/duplicates
	t.Run("GetQualityDuplicates", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/quality/duplicates?limit=10", nil)
		w := httptest.NewRecorder()

		srv.ServeHTTP(w, req)

		if w.Code != http.StatusOK && w.Code != http.StatusNotFound {
			t.Errorf("Expected status 200 or 404, got %d. Body: %s", w.Code, w.Body.String())
		}
	})

	// Тест POST /api/quality/assess
	t.Run("AssessQuality", func(t *testing.T) {
		reqBody := map[string]interface{}{
			"upload_id": 1,
		}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/api/quality/assess", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		srv.ServeHTTP(w, req)

		// Может быть 200, 400 или 404
		if w.Code != http.StatusOK && w.Code != http.StatusBadRequest && w.Code != http.StatusNotFound {
			t.Errorf("Expected status 200, 400 or 404, got %d. Body: %s", w.Code, w.Body.String())
		}
	})
}

// TestClassificationEndpointsExtended тестирует эндпоинты классификации (расширенные)
// Переименовано из TestClassificationEndpoints для избежания конфликта
func TestClassificationEndpointsExtended(t *testing.T) {
	db, err := database.NewDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test DB: %v", err)
	}
	defer db.Close()

	serviceDB, err := database.NewServiceDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create service DB: %v", err)
	}
	defer serviceDB.Close()

	config := &server.Config{
		Port:                    "8080",
		DatabasePath:            ":memory:",
		NormalizedDatabasePath: ":memory:",
		ServiceDatabasePath:     ":memory:",
		LogBufferSize:           100,
		NormalizerEventsBufferSize: 100,
	}

	srv := server.NewServerWithConfig(db, db, serviceDB, ":memory:", ":memory:", config)

	// Тест POST /api/classification/classify
	t.Run("ClassifyItem", func(t *testing.T) {
		reqBody := map[string]interface{}{
			"name": "Молоток большой",
		}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/api/classification/classify", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		srv.ServeHTTP(w, req)

		if w.Code != http.StatusOK && w.Code != http.StatusBadRequest {
			t.Errorf("Expected status 200 or 400, got %d. Body: %s", w.Code, w.Body.String())
		}
	})

	// Тест GET /api/classification/strategies
	t.Run("GetStrategies", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/classification/strategies", nil)
		w := httptest.NewRecorder()

		srv.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}
	})

	// Тест GET /api/classification/available
	t.Run("GetAvailableStrategies", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/classification/available", nil)
		w := httptest.NewRecorder()

		srv.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}
	})
}

// TestNormalizationEndpoints тестирует эндпоинты нормализации
func TestNormalizationEndpoints(t *testing.T) {
	db, err := database.NewDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test DB: %v", err)
	}
	defer db.Close()

	serviceDB, err := database.NewServiceDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create service DB: %v", err)
	}
	defer serviceDB.Close()

	config := &server.Config{
		Port:                    "8080",
		DatabasePath:            ":memory:",
		NormalizedDatabasePath: ":memory:",
		ServiceDatabasePath:     ":memory:",
		LogBufferSize:           100,
		NormalizerEventsBufferSize: 100,
	}

	srv := server.NewServerWithConfig(db, db, serviceDB, ":memory:", ":memory:", config)

	// Тест GET /api/normalization/status
	t.Run("GetNormalizationStatus", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/normalization/status", nil)
		w := httptest.NewRecorder()

		srv.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}
	})

	// Тест GET /api/normalization/stats
	t.Run("GetNormalizationStats", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/normalization/stats", nil)
		w := httptest.NewRecorder()

		srv.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}
	})

	// Тест GET /api/normalization/groups
	t.Run("GetNormalizationGroups", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/normalization/groups?limit=10", nil)
		w := httptest.NewRecorder()

		srv.ServeHTTP(w, req)

		if w.Code != http.StatusOK && w.Code != http.StatusNotFound {
			t.Errorf("Expected status 200 or 404, got %d. Body: %s", w.Code, w.Body.String())
		}
	})

	// Тест POST /api/normalization/stop
	t.Run("StopNormalization", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/normalization/stop", nil)
		w := httptest.NewRecorder()

		srv.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}
	})
}

// TestPatternEndpoints тестирует эндпоинты для работы с паттернами
func TestPatternEndpoints(t *testing.T) {
	db, err := database.NewDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test DB: %v", err)
	}
	defer db.Close()

	serviceDB, err := database.NewServiceDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create service DB: %v", err)
	}
	defer serviceDB.Close()

	config := &server.Config{
		Port:                    "8080",
		DatabasePath:            ":memory:",
		NormalizedDatabasePath: ":memory:",
		ServiceDatabasePath:     ":memory:",
		LogBufferSize:           100,
		NormalizerEventsBufferSize: 100,
	}

	srv := server.NewServerWithConfig(db, db, serviceDB, ":memory:", ":memory:", config)

	// Тест POST /api/patterns/detect
	t.Run("DetectPatterns", func(t *testing.T) {
		reqBody := map[string]interface{}{
			"name": "Товар ER-00013004 100x100",
		}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/api/patterns/detect", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		srv.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}

		var response map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Errorf("Failed to parse response: %v", err)
		}
	})

	// Тест POST /api/patterns/suggest
	t.Run("SuggestPatterns", func(t *testing.T) {
		reqBody := map[string]interface{}{
			"name": "Товар ER-00013004",
		}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/api/patterns/suggest", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		srv.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}
	})
}

// TestErrorCases тестирует обработку ошибок
func TestErrorCases(t *testing.T) {
	db, err := database.NewDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test DB: %v", err)
	}
	defer db.Close()

	serviceDB, err := database.NewServiceDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create service DB: %v", err)
	}
	defer serviceDB.Close()

	config := &server.Config{
		Port:                    "8080",
		DatabasePath:            ":memory:",
		NormalizedDatabasePath: ":memory:",
		ServiceDatabasePath:     ":memory:",
		LogBufferSize:           100,
		NormalizerEventsBufferSize: 100,
	}

	srv := server.NewServerWithConfig(db, db, serviceDB, ":memory:", ":memory:", config)

	// Тест 404 для несуществующего эндпоинта
	t.Run("NotFoundEndpoint", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/nonexistent", nil)
		w := httptest.NewRecorder()

		srv.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Errorf("Expected status 404, got %d", w.Code)
		}
	})

	// Тест 400 для невалидного JSON
	t.Run("InvalidJSON", func(t *testing.T) {
		req := httptest.NewRequest("POST", "/api/classification/classify", bytes.NewReader([]byte("invalid json")))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		srv.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d. Body: %s", w.Code, w.Body.String())
		}
	})

	// Тест 400 для отсутствующих обязательных полей
	t.Run("MissingRequiredFields", func(t *testing.T) {
		reqBody := map[string]interface{}{}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/api/normalization/start", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		srv.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d. Body: %s", w.Code, w.Body.String())
		}
	})

	// Тест неподдерживаемого метода
	t.Run("MethodNotAllowed", func(t *testing.T) {
		req := httptest.NewRequest("DELETE", "/api/quality/stats", nil)
		w := httptest.NewRecorder()

		srv.ServeHTTP(w, req)

		// Может быть 404 или 405 в зависимости от реализации
		if w.Code != http.StatusNotFound && w.Code != http.StatusMethodNotAllowed {
			t.Errorf("Expected status 404 or 405, got %d", w.Code)
		}
	})
}

// TestHealthEndpoints тестирует health check эндпоинты
func TestHealthEndpoints(t *testing.T) {
	db, err := database.NewDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test DB: %v", err)
	}
	defer db.Close()

	serviceDB, err := database.NewServiceDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create service DB: %v", err)
	}
	defer serviceDB.Close()

	config := &server.Config{
		Port:                    "8080",
		DatabasePath:            ":memory:",
		NormalizedDatabasePath: ":memory:",
		ServiceDatabasePath:     ":memory:",
		LogBufferSize:           100,
		NormalizerEventsBufferSize: 100,
	}

	srv := server.NewServerWithConfig(db, db, serviceDB, ":memory:", ":memory:", config)

	// Тест GET /health
	t.Run("HealthCheck", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/health", nil)
		w := httptest.NewRecorder()

		srv.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}
	})

	// Тест GET /api/v1/health
	t.Run("HealthCheckV1", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/v1/health", nil)
		w := httptest.NewRecorder()

		srv.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}
	})
}

// TestWorkerConfigEndpoints тестирует эндпоинты управления воркерами и моделями
func TestWorkerConfigEndpoints(t *testing.T) {
	db, err := database.NewDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test DB: %v", err)
	}
	defer db.Close()

	serviceDB, err := database.NewServiceDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create service DB: %v", err)
	}
	defer serviceDB.Close()

	config := &server.Config{
		Port:                    "8080",
		DatabasePath:            ":memory:",
		NormalizedDatabasePath: ":memory:",
		ServiceDatabasePath:     ":memory:",
		LogBufferSize:           100,
		NormalizerEventsBufferSize: 100,
	}

	srv := server.NewServerWithConfig(db, db, serviceDB, ":memory:", ":memory:", config)

	// Тест GET /api/workers/config
	t.Run("GetWorkerConfig", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/workers/config", nil)
		w := httptest.NewRecorder()

		srv.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}

		var response map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Errorf("Failed to parse response: %v", err)
		}

		if _, ok := response["providers"]; !ok {
			t.Error("Response should contain 'providers' field")
		}
		if _, ok := response["default_provider"]; !ok {
			t.Error("Response should contain 'default_provider' field")
		}
	})

	// Тест POST /api/workers/config/update - set_max_workers
	t.Run("UpdateMaxWorkers", func(t *testing.T) {
		reqBody := map[string]interface{}{
			"action": "set_max_workers",
			"data": map[string]interface{}{
				"max_workers": 4,
			},
		}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/api/workers/config/update", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		srv.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}
	})

	// Тест GET /api/workers/providers
	t.Run("GetAvailableProviders", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/workers/providers", nil)
		w := httptest.NewRecorder()

		srv.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}

		var response map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Errorf("Failed to parse response: %v", err)
		}

		if _, ok := response["providers"]; !ok {
			t.Error("Response should contain 'providers' field")
		}
	})

	// Тест POST /api/workers/config/update - unknown action
	t.Run("UnknownAction", func(t *testing.T) {
		reqBody := map[string]interface{}{
			"action": "unknown_action",
			"data":   map[string]interface{}{},
		}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/api/workers/config/update", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		srv.ServeHTTP(w, req)

		if w.Code != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d. Body: %s", w.Code, w.Body.String())
		}
	})
}

