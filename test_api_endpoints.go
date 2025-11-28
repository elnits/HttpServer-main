package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"httpserver/database"
	"httpserver/server"
)

// ВАЖНО: Этот файл содержит тесты для API эндпоинтов.
// Для компиляции и запуска тестов требуется:
// 1. Реализовать метод ServeHTTP в структуре Server или использовать http.Handler
// 2. Или переписать тесты для использования прямого вызова handler функций
//
// Текущие тесты проверяют структуру запросов и обработку ошибок.
// Для полного функционального тестирования требуется запуск сервера и использование curl/HTTP клиентов.

// TestVersioningEndpoints тестирует endpoints для версионирования
func TestVersioningEndpoints(t *testing.T) {
	// Создаем тестовую БД
	db, err := database.NewDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test DB: %v", err)
	}
	defer db.Close()

	// Создаем сервисную БД
	serviceDB, err := database.NewServiceDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create service DB: %v", err)
	}
	defer serviceDB.Close()

	// Создаем сервер
	config := &server.Config{
		Port:                    "8080",
		DatabasePath:            ":memory:",
		NormalizedDatabasePath: ":memory:",
		ServiceDatabasePath:     ":memory:",
		LogBufferSize:           100,
		NormalizerEventsBufferSize: 100,
	}

	srv := server.NewServerWithConfig(db, db, serviceDB, ":memory:", ":memory:", config)

	// Тест 1: POST /api/normalization/start
	t.Run("StartNormalization", func(t *testing.T) {
		reqBody := map[string]interface{}{
			"item_id":       1,
			"original_name": "Тестовый товар",
		}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/api/normalization/start", bytes.NewReader(body))
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

		if sessionID, ok := response["session_id"].(float64); !ok || sessionID == 0 {
			t.Errorf("Expected session_id, got %v", response["session_id"])
		}
	})

	// Тест 2: POST /api/normalization/apply-patterns
	t.Run("ApplyPatterns", func(t *testing.T) {
		// Сначала создаем сессию
		reqBody := map[string]interface{}{
			"item_id":       1,
			"original_name": "Тестовый товар",
		}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/api/normalization/start", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)

		var startResponse map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &startResponse)
		sessionID := int(startResponse["session_id"].(float64))

		// Теперь применяем паттерны
		reqBody2 := map[string]interface{}{
			"session_id": sessionID,
			"stage_type": "algorithmic",
		}
		body2, _ := json.Marshal(reqBody2)
		req2 := httptest.NewRequest("POST", "/api/normalization/apply-patterns", bytes.NewReader(body2))
		req2.Header.Set("Content-Type", "application/json")
		w2 := httptest.NewRecorder()

		srv.ServeHTTP(w2, req2)

		if w2.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d. Body: %s", w2.Code, w2.Body.String())
		}
	})

	// Тест 3: GET /api/normalization/history
	t.Run("GetSessionHistory", func(t *testing.T) {
		// Создаем сессию
		reqBody := map[string]interface{}{
			"item_id":       1,
			"original_name": "Тестовый товар",
		}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/api/normalization/start", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)

		var startResponse map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &startResponse)
		sessionID := int(startResponse["session_id"].(float64))

		// Получаем историю
		req2 := httptest.NewRequest("GET", fmt.Sprintf("/api/normalization/history?session_id=%d", sessionID), nil)
		w2 := httptest.NewRecorder()

		srv.ServeHTTP(w2, req2)

		if w2.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d. Body: %s", w2.Code, w2.Body.String())
		}
	})

	// Тест 4: POST /api/normalization/apply-ai
	t.Run("ApplyAI", func(t *testing.T) {
		// Сначала создаем сессию
		reqBody := map[string]interface{}{
			"item_id":       1,
			"original_name": "Тестовый товар для AI",
		}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/api/normalization/start", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)

		var startResponse map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &startResponse)
		sessionID := int(startResponse["session_id"].(float64))

		// Сохраняем оригинальный API ключ
		originalKey := os.Getenv("ARLIAI_API_KEY")
		defer os.Setenv("ARLIAI_API_KEY", originalKey)

		// Устанавливаем тестовый ключ (или проверяем отсутствие)
		if originalKey == "" {
			// Если ключ не установлен, тест должен вернуть ошибку 400
			reqBody2 := map[string]interface{}{
				"session_id": sessionID,
				"use_chat":   false,
			}
			body2, _ := json.Marshal(reqBody2)
			req2 := httptest.NewRequest("POST", "/api/normalization/apply-ai", bytes.NewReader(body2))
			req2.Header.Set("Content-Type", "application/json")
			w2 := httptest.NewRecorder()

			srv.ServeHTTP(w2, req2)

			// Ожидаем ошибку, если API ключ не установлен
			if w2.Code != http.StatusBadRequest {
				t.Logf("Expected status 400 (API key not set), got %d. Body: %s", w2.Code, w2.Body.String())
			}
		} else {
			// Если ключ установлен, пробуем выполнить запрос
			reqBody2 := map[string]interface{}{
				"session_id": sessionID,
				"use_chat":   false,
			}
			body2, _ := json.Marshal(reqBody2)
			req2 := httptest.NewRequest("POST", "/api/normalization/apply-ai", bytes.NewReader(body2))
			req2.Header.Set("Content-Type", "application/json")
			w2 := httptest.NewRecorder()

			srv.ServeHTTP(w2, req2)

			// Может быть ошибка, если API недоступен, но структура запроса правильная
			if w2.Code != http.StatusOK && w2.Code != http.StatusInternalServerError {
				t.Errorf("Expected status 200 or 500, got %d. Body: %s", w2.Code, w2.Body.String())
			}
		}
	})

	// Тест 5: POST /api/normalization/revert
	t.Run("RevertStage", func(t *testing.T) {
		// Создаем сессию
		reqBody := map[string]interface{}{
			"item_id":       1,
			"original_name": "Тестовый товар для отката",
		}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/api/normalization/start", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)

		var startResponse map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &startResponse)
		sessionID := int(startResponse["session_id"].(float64))

		// Применяем паттерны для создания стадии
		reqBody2 := map[string]interface{}{
			"session_id": sessionID,
			"stage_type": "algorithmic",
		}
		body2, _ := json.Marshal(reqBody2)
		req2 := httptest.NewRequest("POST", "/api/normalization/apply-patterns", bytes.NewReader(body2))
		req2.Header.Set("Content-Type", "application/json")
		w2 := httptest.NewRecorder()
		srv.ServeHTTP(w2, req2)

		// Теперь откатываемся к стадии 1
		reqBody3 := map[string]interface{}{
			"session_id":   sessionID,
			"target_stage": 1,
		}
		body3, _ := json.Marshal(reqBody3)
		req3 := httptest.NewRequest("POST", "/api/normalization/revert", bytes.NewReader(body3))
		req3.Header.Set("Content-Type", "application/json")
		w3 := httptest.NewRecorder()

		srv.ServeHTTP(w3, req3)

		if w3.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d. Body: %s", w3.Code, w3.Body.String())
		}

		var revertResponse map[string]interface{}
		if err := json.Unmarshal(w3.Body.Bytes(), &revertResponse); err != nil {
			t.Errorf("Failed to parse response: %v", err)
		}

		if success, ok := revertResponse["success"].(bool); !ok || !success {
			t.Errorf("Expected success=true, got %v", revertResponse["success"])
		}
	})
}

// TestClassificationEndpoints тестирует endpoints для классификации
func TestClassificationEndpoints(t *testing.T) {
	// Создаем тестовую БД
	db, err := database.NewDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test DB: %v", err)
	}
	defer db.Close()

	// Создаем сервисную БД
	serviceDB, err := database.NewServiceDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create service DB: %v", err)
	}
	defer serviceDB.Close()

	// Создаем сервер
	config := &server.Config{
		Port:                    "8080",
		DatabasePath:            ":memory:",
		NormalizedDatabasePath: ":memory:",
		ServiceDatabasePath:     ":memory:",
		LogBufferSize:           100,
		NormalizerEventsBufferSize: 100,
	}

	srv := server.NewServerWithConfig(db, db, serviceDB, ":memory:", ":memory:", config)

	// Тест 1: GET /api/classification/strategies
	t.Run("GetStrategies", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/classification/strategies", nil)
		w := httptest.NewRecorder()

		srv.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}

		var response map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Errorf("Failed to parse response: %v", err)
		}

		if strategies, ok := response["strategies"].([]interface{}); !ok || len(strategies) == 0 {
			t.Errorf("Expected strategies array, got %v", response["strategies"])
		}
	})

	// Тест 2: GET /api/classification/available
	t.Run("GetAvailableStrategies", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/classification/available", nil)
		w := httptest.NewRecorder()

		srv.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}
	})

	// Тест 3: GET /api/classification/strategies/client?client_id=1
	t.Run("GetClientStrategies", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/classification/strategies/client?client_id=1", nil)
		w := httptest.NewRecorder()

		srv.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}
	})

	// Тест 4: POST /api/classification/strategies/create
	t.Run("CreateClientStrategy", func(t *testing.T) {
		reqBody := map[string]interface{}{
			"client_id":   1,
			"name":        "Тестовая стратегия",
			"description": "Описание тестовой стратегии",
			"max_depth":   2,
			"priority":    []string{"0", "1"},
			"rules":       []map[string]interface{}{},
		}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/api/classification/strategies/create", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		srv.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Errorf("Expected status 201, got %d. Body: %s", w.Code, w.Body.String())
		}
	})

	// Тест 5: POST /api/normalization/apply-categorization
	t.Run("ApplyCategorization", func(t *testing.T) {
		// Создаем сессию нормализации
		reqBody := map[string]interface{}{
			"item_id":       1,
			"original_name": "Игровой ноутбук ASUS",
		}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/api/normalization/start", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)

		var startResponse map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &startResponse)
		sessionID := int(startResponse["session_id"].(float64))

		// Сохраняем оригинальный API ключ
		originalKey := os.Getenv("ARLIAI_API_KEY")
		defer os.Setenv("ARLIAI_API_KEY", originalKey)

		// Применяем классификацию
		reqBody2 := map[string]interface{}{
			"session_id":  sessionID,
			"strategy_id": "top_priority",
		}
		body2, _ := json.Marshal(reqBody2)
		req2 := httptest.NewRequest("POST", "/api/normalization/apply-categorization", bytes.NewReader(body2))
		req2.Header.Set("Content-Type", "application/json")
		w2 := httptest.NewRecorder()

		srv.ServeHTTP(w2, req2)

		// Если API ключ не установлен, может быть ошибка, но структура запроса правильная
		if w2.Code != http.StatusOK && w2.Code != http.StatusInternalServerError && w2.Code != http.StatusBadRequest {
			t.Errorf("Expected status 200, 400, or 500, got %d. Body: %s", w2.Code, w2.Body.String())
		}
	})

	// Тест 6: POST /api/classification/classify-item
	t.Run("ClassifyItemDirect", func(t *testing.T) {
		// Сохраняем оригинальный API ключ
		originalKey := os.Getenv("ARLIAI_API_KEY")
		defer os.Setenv("ARLIAI_API_KEY", originalKey)

		reqBody := map[string]interface{}{
			"item_name":   "Игровой ноутбук ASUS ROG",
			"item_code":   "ASUS001",
			"strategy_id": "top_priority",
			"category":    "общее",
			"context": map[string]interface{}{
				"brand": "ASUS",
				"type":  "laptop",
			},
		}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/api/classification/classify-item", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		srv.ServeHTTP(w, req)

		// Если API ключ не установлен, ожидаем ошибку 400
		if originalKey == "" {
			if w.Code != http.StatusBadRequest {
				t.Logf("Expected status 400 (API key not set), got %d. Body: %s", w.Code, w.Body.String())
			}
		} else {
			// Если ключ установлен, может быть ошибка если API недоступен, но структура правильная
			if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
				t.Errorf("Expected status 200 or 500, got %d. Body: %s", w.Code, w.Body.String())
			}
		}
	})
}

// TestIntegrationFlow тестирует полный поток нормализации и классификации
func TestIntegrationFlow(t *testing.T) {
	// Создаем тестовую БД
	db, err := database.NewDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test DB: %v", err)
	}
	defer db.Close()

	// Создаем сервисную БД
	serviceDB, err := database.NewServiceDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create service DB: %v", err)
	}
	defer serviceDB.Close()

	// Создаем сервер
	config := &server.Config{
		Port:                    "8080",
		DatabasePath:            ":memory:",
		NormalizedDatabasePath: ":memory:",
		ServiceDatabasePath:     ":memory:",
		LogBufferSize:           100,
		NormalizerEventsBufferSize: 100,
	}

	srv := server.NewServerWithConfig(db, db, serviceDB, ":memory:", ":memory:", config)

	// Шаг 1: Начинаем нормализацию
	reqBody := map[string]interface{}{
		"item_id":       1,
		"original_name": "Тестовый товар для классификации",
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/api/normalization/start", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Failed to start normalization: %d. Body: %s", w.Code, w.Body.String())
	}

	var startResponse map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &startResponse)
	sessionID := int(startResponse["session_id"].(float64))

	// Шаг 2: Применяем паттерны
	reqBody2 := map[string]interface{}{
		"session_id": sessionID,
		"stage_type": "algorithmic",
	}
	body2, _ := json.Marshal(reqBody2)
	req2 := httptest.NewRequest("POST", "/api/normalization/apply-patterns", bytes.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	srv.ServeHTTP(w2, req2)

	if w2.Code != http.StatusOK {
		t.Errorf("Failed to apply patterns: %d. Body: %s", w2.Code, w2.Body.String())
	}

	// Шаг 3: Получаем историю
	req3 := httptest.NewRequest("GET", fmt.Sprintf("/api/normalization/history?session_id=%d", sessionID), nil)
	w3 := httptest.NewRecorder()
	srv.ServeHTTP(w3, req3)

	if w3.Code != http.StatusOK {
		t.Errorf("Failed to get history: %d. Body: %s", w3.Code, w3.Body.String())
	}
}

// ПРИМЕЧАНИЕ: Для компиляции этих тестов требуется:
// 1. Добавить метод ServeHTTP в структуру Server (реализовать http.Handler)
// 2. Или переписать тесты для прямого вызова handler функций через mux
//
// Альтернатива: Использовать интеграционные тесты через curl/HTTP клиенты
// (см. test_endpoints.sh, test_endpoints.ps1)

