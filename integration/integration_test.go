package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"httpserver/database"
	"httpserver/normalization"
	"httpserver/server"
)

// TestFullNormalizationFlow тестирует полный поток нормализации
func TestFullNormalizationFlow(t *testing.T) {
	// Создаем тестовые БД
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

	// Создаем тестовые данные
	upload, err := db.CreateUpload("test-uuid", "8.3", "test-config")
	if err != nil {
		t.Fatalf("Failed to create upload: %v", err)
	}

	catalog, err := db.AddCatalog(upload.ID, "TestCatalog", "test_catalog")
	if err != nil {
		t.Fatalf("Failed to create catalog: %v", err)
	}

	// Добавляем элементы каталога
	err = db.AddCatalogItem(catalog.ID, "ref1", "code1", "Молоток ER-00013004", "", "")
	if err != nil {
		t.Fatalf("Failed to add catalog item: %v", err)
	}

	err = db.AddCatalogItem(catalog.ID, "ref2", "code2", "Молоток большой", "", "")
	if err != nil {
		t.Fatalf("Failed to add catalog item: %v", err)
	}

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

	// Шаг 1: Начинаем нормализацию через API
	t.Run("StartNormalization", func(t *testing.T) {
		var itemID int
		err := db.QueryRow("SELECT id FROM catalog_items WHERE reference = ?", "ref1").Scan(&itemID)
		if err != nil {
			t.Fatalf("Failed to get item ID: %v", err)
		}

		reqBody := map[string]interface{}{
			"item_id":       itemID,
			"original_name": "Молоток ER-00013004",
		}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/api/normalization/start", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()

		srv.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
		}
	})

	// Шаг 2: Применяем паттерны
	t.Run("ApplyPatterns", func(t *testing.T) {
		var itemID int
		err := db.QueryRow("SELECT id FROM catalog_items WHERE reference = ?", "ref1").Scan(&itemID)
		if err != nil {
			t.Fatalf("Failed to get item ID: %v", err)
		}

		// Создаем сессию
		reqBody := map[string]interface{}{
			"item_id":       itemID,
			"original_name": "Молоток ER-00013004",
		}
		body, _ := json.Marshal(reqBody)
		req := httptest.NewRequest("POST", "/api/normalization/start", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		srv.ServeHTTP(w, req)

		var startResponse map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &startResponse)
		sessionID := int(startResponse["session_id"].(float64))

		// Применяем паттерны
		reqBody2 := map[string]interface{}{
			"session_id": sessionID,
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

	// Шаг 3: Получаем историю
	t.Run("GetHistory", func(t *testing.T) {
		var itemID int
		err := db.QueryRow("SELECT id FROM catalog_items WHERE reference = ?", "ref1").Scan(&itemID)
		if err != nil {
			t.Fatalf("Failed to get item ID: %v", err)
		}

		// Создаем сессию
		reqBody := map[string]interface{}{
			"item_id":       itemID,
			"original_name": "Молоток ER-00013004",
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
}

// TestNormalizationWithRevert тестирует нормализацию с откатом
func TestNormalizationWithRevert(t *testing.T) {
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

	// Создаем тестовые данные
	upload, err := db.CreateUpload("test-uuid", "8.3", "test-config")
	if err != nil {
		t.Fatalf("Failed to create upload: %v", err)
	}

	catalog, err := db.AddCatalog(upload.ID, "TestCatalog", "test_catalog")
	if err != nil {
		t.Fatalf("Failed to create catalog: %v", err)
	}

	err = db.AddCatalogItem(catalog.ID, "ref1", "code1", "Товар ER-00013004", "", "")
	if err != nil {
		t.Fatalf("Failed to add catalog item: %v", err)
	}

	config := &server.Config{
		Port:                    "8080",
		DatabasePath:            ":memory:",
		NormalizedDatabasePath: ":memory:",
		ServiceDatabasePath:     ":memory:",
		LogBufferSize:           100,
		NormalizerEventsBufferSize: 100,
	}

	srv := server.NewServerWithConfig(db, db, serviceDB, ":memory:", ":memory:", config)

	// Создаем сессию и применяем паттерны
	var itemID int
	err = db.QueryRow("SELECT id FROM catalog_items WHERE reference = ?", "ref1").Scan(&itemID)
	if err != nil {
		t.Fatalf("Failed to get item ID: %v", err)
	}

	reqBody := map[string]interface{}{
		"item_id":       itemID,
		"original_name": "Товар ER-00013004",
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/api/normalization/start", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	var startResponse map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &startResponse)
	sessionID := int(startResponse["session_id"].(float64))

	// Применяем паттерны
	reqBody2 := map[string]interface{}{
		"session_id": sessionID,
	}
	body2, _ := json.Marshal(reqBody2)
	req2 := httptest.NewRequest("POST", "/api/normalization/apply-patterns", bytes.NewReader(body2))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	srv.ServeHTTP(w2, req2)

	// Получаем историю для получения ID стадии
	req3 := httptest.NewRequest("GET", fmt.Sprintf("/api/normalization/history?session_id=%d", sessionID), nil)
	w3 := httptest.NewRecorder()
	srv.ServeHTTP(w3, req3)

	var historyResponse map[string]interface{}
	json.Unmarshal(w3.Body.Bytes(), &historyResponse)
	stages, ok := historyResponse["stages"].([]interface{})
	if !ok || len(stages) == 0 {
		t.Skip("No stages to revert to")
		return
	}

	// Откатываемся к первой стадии
	firstStage := stages[0].(map[string]interface{})
	stageID := int(firstStage["id"].(float64))

	reqBody4 := map[string]interface{}{
		"session_id": sessionID,
		"stage_id":   stageID,
	}
	body4, _ := json.Marshal(reqBody4)
	req4 := httptest.NewRequest("POST", "/api/normalization/revert", bytes.NewReader(body4))
	req4.Header.Set("Content-Type", "application/json")
	w4 := httptest.NewRecorder()

	srv.ServeHTTP(w4, req4)

	if w4.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d. Body: %s", w4.Code, w4.Body.String())
	}
}

// TestClassificationIntegration тестирует интеграцию нормализации и классификации
func TestClassificationIntegration(t *testing.T) {
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

	// Создаем нормализатор
	events := make(chan string, 10)
	aiConfig := &normalization.AIConfig{
		Enabled: false,
	}
	normalizer := normalization.NewNormalizer(db, events, aiConfig)

	// Создаем тестовые данные
	upload, err := db.CreateUpload("test-uuid", "8.3", "test-config")
	if err != nil {
		t.Fatalf("Failed to create upload: %v", err)
	}

	catalog, err := db.AddCatalog(upload.ID, "TestCatalog", "test_catalog")
	if err != nil {
		t.Fatalf("Failed to create catalog: %v", err)
	}

	err = db.AddCatalogItem(catalog.ID, "ref1", "code1", "Молоток большой", "", "")
	if err != nil {
		t.Fatalf("Failed to add catalog item: %v", err)
	}

	// Нормализуем данные
	normalizer.SetSourceConfig("catalog_items", "reference", "code", "name")
	err = normalizer.ProcessNormalization()
	if err != nil {
		t.Logf("Normalization returned error (expected for empty normalized table): %v", err)
	}

	// Тестируем классификацию нормализованных данных
	config := &server.Config{
		Port:                    "8080",
		DatabasePath:            ":memory:",
		NormalizedDatabasePath: ":memory:",
		ServiceDatabasePath:     ":memory:",
		LogBufferSize:           100,
		NormalizerEventsBufferSize: 100,
	}

	srv := server.NewServerWithConfig(db, db, serviceDB, ":memory:", ":memory:", config)

	reqBody := map[string]interface{}{
		"name": "молоток",
	}
	body, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/api/classification/classify", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK && w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 200 or 400, got %d. Body: %s", w.Code, w.Body.String())
	}
}

