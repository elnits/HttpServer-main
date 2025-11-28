package normalization

import (
	"testing"

	"httpserver/database"
)

func TestNewVersionedPipeline(t *testing.T) {
	db, err := database.NewDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test DB: %v", err)
	}
	defer db.Close()

	patternDetector := NewPatternDetector()
	aiNormalizer := NewAINormalizer("test-key")
	aiIntegrator := NewPatternAIIntegrator(patternDetector, aiNormalizer)

	pipeline := NewVersionedNormalizationPipeline(db, patternDetector, aiIntegrator)

	if pipeline == nil {
		t.Error("NewVersionedNormalizationPipeline returned nil")
	}
	if pipeline.db != db {
		t.Error("Database not set correctly")
	}
	if pipeline.patternDetector != patternDetector {
		t.Error("PatternDetector not set correctly")
	}
	if pipeline.aiIntegrator != aiIntegrator {
		t.Error("AIIntegrator not set correctly")
	}
	if pipeline.metadata == nil {
		t.Error("Metadata map not initialized")
	}
	if pipeline.stages == nil {
		t.Error("Stages slice not initialized")
	}
}

func TestStartSession(t *testing.T) {
	db, err := database.NewDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test DB: %v", err)
	}
	defer db.Close()

	// Создаем тестовые данные
	upload, err := db.CreateUpload("test-uuid", "8.3", "test-config")
	if err != nil {
		t.Fatalf("Failed to create upload: %v", err)
	}

	catalog, err := db.AddCatalog(upload.ID, "TestCatalog", "test_catalog")
	if err != nil {
		t.Fatalf("Failed to create catalog: %v", err)
	}

	err = db.AddCatalogItem(catalog.ID, "ref1", "code1", "Test Item", "", "")
	if err != nil {
		t.Fatalf("Failed to create catalog item: %v", err)
	}

	var itemID int
	err = db.QueryRow("SELECT id FROM catalog_items WHERE reference = ?", "ref1").Scan(&itemID)
	if err != nil {
		t.Fatalf("Failed to get catalog item ID: %v", err)
	}

	patternDetector := NewPatternDetector()
	aiNormalizer := NewAINormalizer("test-key")
	aiIntegrator := NewPatternAIIntegrator(patternDetector, aiNormalizer)
	pipeline := NewVersionedNormalizationPipeline(db, patternDetector, aiIntegrator)

	err = pipeline.StartSession(itemID, "Original Name")
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}

	if pipeline.sessionID == 0 {
		t.Error("Session ID not set")
	}
	if pipeline.catalogItemID != itemID {
		t.Errorf("Expected catalogItemID %d, got %d", itemID, pipeline.catalogItemID)
	}
	if pipeline.originalName != "Original Name" {
		t.Errorf("Expected originalName 'Original Name', got '%s'", pipeline.originalName)
	}
	if pipeline.currentName != "Original Name" {
		t.Errorf("Expected currentName 'Original Name', got '%s'", pipeline.currentName)
	}
}

func TestVersionedPipelineApplyPatterns(t *testing.T) {
	db, err := database.NewDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test DB: %v", err)
	}
	defer db.Close()

	// Создаем тестовые данные
	upload, err := db.CreateUpload("test-uuid", "8.3", "test-config")
	if err != nil {
		t.Fatalf("Failed to create upload: %v", err)
	}

	catalog, err := db.AddCatalog(upload.ID, "TestCatalog", "test_catalog")
	if err != nil {
		t.Fatalf("Failed to create catalog: %v", err)
	}

	err = db.AddCatalogItem(catalog.ID, "ref1", "code1", "Test Item ER-00013004", "", "")
	if err != nil {
		t.Fatalf("Failed to create catalog item: %v", err)
	}

	var itemID int
	err = db.QueryRow("SELECT id FROM catalog_items WHERE reference = ?", "ref1").Scan(&itemID)
	if err != nil {
		t.Fatalf("Failed to get catalog item ID: %v", err)
	}

	patternDetector := NewPatternDetector()
	aiNormalizer := NewAINormalizer("test-key")
	aiIntegrator := NewPatternAIIntegrator(patternDetector, aiNormalizer)
	pipeline := NewVersionedNormalizationPipeline(db, patternDetector, aiIntegrator)

	err = pipeline.StartSession(itemID, "Test Item ER-00013004")
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}

	err = pipeline.ApplyPatterns()
	if err != nil {
		t.Fatalf("ApplyPatterns failed: %v", err)
	}

	// Проверяем, что имя изменилось (технический код должен быть удален)
	currentName := pipeline.GetCurrentName()
	if currentName == "Test Item ER-00013004" {
		t.Error("Patterns were not applied - technical code still present")
	}

	// Проверяем историю
	history, err := pipeline.GetHistory()
	if err != nil {
		t.Fatalf("GetHistory failed: %v", err)
	}
	if len(history) == 0 {
		t.Error("Expected at least one stage in history")
	}
}

func TestGetHistory(t *testing.T) {
	db, err := database.NewDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test DB: %v", err)
	}
	defer db.Close()

	// Создаем тестовые данные
	upload, err := db.CreateUpload("test-uuid", "8.3", "test-config")
	if err != nil {
		t.Fatalf("Failed to create upload: %v", err)
	}

	catalog, err := db.AddCatalog(upload.ID, "TestCatalog", "test_catalog")
	if err != nil {
		t.Fatalf("Failed to create catalog: %v", err)
	}

	err = db.AddCatalogItem(catalog.ID, "ref1", "code1", "Test Item", "", "")
	if err != nil {
		t.Fatalf("Failed to create catalog item: %v", err)
	}

	var itemID int
	err = db.QueryRow("SELECT id FROM catalog_items WHERE reference = ?", "ref1").Scan(&itemID)
	if err != nil {
		t.Fatalf("Failed to get catalog item ID: %v", err)
	}

	patternDetector := NewPatternDetector()
	aiNormalizer := NewAINormalizer("test-key")
	aiIntegrator := NewPatternAIIntegrator(patternDetector, aiNormalizer)
	pipeline := NewVersionedNormalizationPipeline(db, patternDetector, aiIntegrator)

	// Тест без сессии
	_, err = pipeline.GetHistory()
	if err == nil {
		t.Error("Expected error when getting history without session")
	}

	// Создаем сессию
	err = pipeline.StartSession(itemID, "Test Item")
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}

	// Получаем историю (должна быть пустой)
	history, err := pipeline.GetHistory()
	if err != nil {
		t.Fatalf("GetHistory failed: %v", err)
	}
	if len(history) != 0 {
		t.Errorf("Expected empty history, got %d stages", len(history))
	}

	// Применяем паттерны
	err = pipeline.ApplyPatterns()
	if err != nil {
		t.Fatalf("ApplyPatterns failed: %v", err)
	}

	// Получаем историю снова
	history, err = pipeline.GetHistory()
	if err != nil {
		t.Fatalf("GetHistory failed: %v", err)
	}
	if len(history) == 0 {
		t.Error("Expected at least one stage in history after ApplyPatterns")
	}
}

func TestSessionNotFound(t *testing.T) {
	db, err := database.NewDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test DB: %v", err)
	}
	defer db.Close()

	patternDetector := NewPatternDetector()
	aiNormalizer := NewAINormalizer("test-key")
	aiIntegrator := NewPatternAIIntegrator(patternDetector, aiNormalizer)
	pipeline := NewVersionedNormalizationPipeline(db, patternDetector, aiIntegrator)

	// Пытаемся применить паттерны без сессии
	err = pipeline.ApplyPatterns()
	if err == nil {
		t.Error("Expected error when applying patterns without session")
	}

	// Пытаемся откатиться без сессии
	err = pipeline.RevertToStage(1)
	if err == nil {
		t.Error("Expected error when reverting without session")
	}
}

func TestGetCurrentName(t *testing.T) {
	db, err := database.NewDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test DB: %v", err)
	}
	defer db.Close()

	patternDetector := NewPatternDetector()
	aiNormalizer := NewAINormalizer("test-key")
	aiIntegrator := NewPatternAIIntegrator(patternDetector, aiNormalizer)
	pipeline := NewVersionedNormalizationPipeline(db, patternDetector, aiIntegrator)

	// До создания сессии должно быть пусто
	name := pipeline.GetCurrentName()
	if name != "" {
		t.Errorf("Expected empty name before session, got '%s'", name)
	}

	// Создаем тестовые данные
	upload, err := db.CreateUpload("test-uuid", "8.3", "test-config")
	if err != nil {
		t.Fatalf("Failed to create upload: %v", err)
	}

	catalog, err := db.AddCatalog(upload.ID, "TestCatalog", "test_catalog")
	if err != nil {
		t.Fatalf("Failed to create catalog: %v", err)
	}

	err = db.AddCatalogItem(catalog.ID, "ref1", "code1", "Test Item", "", "")
	if err != nil {
		t.Fatalf("Failed to create catalog item: %v", err)
	}

	var itemID int
	err = db.QueryRow("SELECT id FROM catalog_items WHERE reference = ?", "ref1").Scan(&itemID)
	if err != nil {
		t.Fatalf("Failed to get catalog item ID: %v", err)
	}

	err = pipeline.StartSession(itemID, "Original Name")
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}

	name = pipeline.GetCurrentName()
	if name != "Original Name" {
		t.Errorf("Expected 'Original Name', got '%s'", name)
	}
}

func TestMetadata(t *testing.T) {
	db, err := database.NewDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test DB: %v", err)
	}
	defer db.Close()

	patternDetector := NewPatternDetector()
	aiNormalizer := NewAINormalizer("test-key")
	aiIntegrator := NewPatternAIIntegrator(patternDetector, aiNormalizer)
	pipeline := NewVersionedNormalizationPipeline(db, patternDetector, aiIntegrator)

	// Устанавливаем метаданные
	pipeline.SetMetadata("key1", "value1")
	pipeline.SetMetadata("key2", 123)

	// Получаем метаданные
	value1 := pipeline.GetMetadata("key1")
	if value1 != "value1" {
		t.Errorf("Expected 'value1', got '%v'", value1)
	}

	value2 := pipeline.GetMetadata("key2")
	if value2 != 123 {
		t.Errorf("Expected 123, got '%v'", value2)
	}

	// Несуществующий ключ
	value3 := pipeline.GetMetadata("nonexistent")
	if value3 != nil {
		t.Errorf("Expected nil for nonexistent key, got '%v'", value3)
	}
}

func TestCompleteSession(t *testing.T) {
	db, err := database.NewDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test DB: %v", err)
	}
	defer db.Close()

	// Создаем тестовые данные
	upload, err := db.CreateUpload("test-uuid", "8.3", "test-config")
	if err != nil {
		t.Fatalf("Failed to create upload: %v", err)
	}

	catalog, err := db.AddCatalog(upload.ID, "TestCatalog", "test_catalog")
	if err != nil {
		t.Fatalf("Failed to create catalog: %v", err)
	}

	err = db.AddCatalogItem(catalog.ID, "ref1", "code1", "Test Item", "", "")
	if err != nil {
		t.Fatalf("Failed to create catalog item: %v", err)
	}

	var itemID int
	err = db.QueryRow("SELECT id FROM catalog_items WHERE reference = ?", "ref1").Scan(&itemID)
	if err != nil {
		t.Fatalf("Failed to get catalog item ID: %v", err)
	}

	patternDetector := NewPatternDetector()
	aiNormalizer := NewAINormalizer("test-key")
	aiIntegrator := NewPatternAIIntegrator(patternDetector, aiNormalizer)
	pipeline := NewVersionedNormalizationPipeline(db, patternDetector, aiIntegrator)

	// Тест без сессии
	err = pipeline.CompleteSession()
	if err == nil {
		t.Error("Expected error when completing session without starting it")
	}

	// Создаем сессию
	err = pipeline.StartSession(itemID, "Test Item")
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}

	// Завершаем сессию
	err = pipeline.CompleteSession()
	if err != nil {
		t.Fatalf("CompleteSession failed: %v", err)
	}
}

