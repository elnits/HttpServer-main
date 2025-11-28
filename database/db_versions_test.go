package database

import (
	"testing"
)

func TestCreateNormalizationSession(t *testing.T) {
	db, err := NewDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create DB: %v", err)
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

	// Создаем сессию
	sessionID, err := db.CreateNormalizationSession(itemID, "Original Name")
	if err != nil {
		t.Fatalf("Failed to create normalization session: %v", err)
	}

	if sessionID == 0 {
		t.Error("Session ID is zero")
	}

	// Проверяем, что сессия создана
	session, err := db.GetNormalizationSession(sessionID)
	if err != nil {
		t.Fatalf("Failed to get session: %v", err)
	}

	if session.CatalogItemID != itemID {
		t.Errorf("Expected catalogItemID %d, got %d", itemID, session.CatalogItemID)
	}
	if session.OriginalName != "Original Name" {
		t.Errorf("Expected originalName 'Original Name', got '%s'", session.OriginalName)
	}
	if session.Status != "in_progress" {
		t.Errorf("Expected status 'in_progress', got '%s'", session.Status)
	}
}

func TestCreateStage(t *testing.T) {
	db, err := NewDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create DB: %v", err)
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

	// Создаем сессию
	sessionID, err := db.CreateNormalizationSession(itemID, "Original Name")
	if err != nil {
		t.Fatalf("Failed to create normalization session: %v", err)
	}

	// Создаем стадию
	stage := &NormalizationStage{
		SessionID:       sessionID,
		StageType:       "algorithmic",
		StageName:       "pattern_correction",
		InputName:       "Original Name",
		OutputName:      "Corrected Name",
		AppliedPatterns: "[]",
		Confidence:      0.95,
		Status:          "applied",
	}

	err = db.AddNormalizationStage(stage)
	if err != nil {
		t.Fatalf("Failed to add normalization stage: %v", err)
	}

	// Проверяем, что стадия создана
	history, err := db.GetSessionHistory(sessionID)
	if err != nil {
		t.Fatalf("Failed to get session history: %v", err)
	}

	if len(history) != 1 {
		t.Errorf("Expected 1 stage, got %d", len(history))
	}

	if history[0].StageType != "algorithmic" {
		t.Errorf("Expected stage type 'algorithmic', got '%s'", history[0].StageType)
	}
	if history[0].OutputName != "Corrected Name" {
		t.Errorf("Expected output name 'Corrected Name', got '%s'", history[0].OutputName)
	}
}

func TestGetSessionHistory(t *testing.T) {
	db, err := NewDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create DB: %v", err)
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

	// Создаем сессию
	sessionID, err := db.CreateNormalizationSession(itemID, "Original Name")
	if err != nil {
		t.Fatalf("Failed to create normalization session: %v", err)
	}

	// Создаем несколько стадий
	for i := 0; i < 3; i++ {
		stage := &NormalizationStage{
			SessionID:       sessionID,
			StageType:       "algorithmic",
			StageName:       "pattern_correction",
			InputName:       "Input",
			OutputName:       "Output",
			AppliedPatterns: "[]",
			Confidence:      0.95,
			Status:          "applied",
		}
		err = db.AddNormalizationStage(stage)
		if err != nil {
			t.Fatalf("Failed to add normalization stage: %v", err)
		}
	}

	// Получаем историю
	history, err := db.GetSessionHistory(sessionID)
	if err != nil {
		t.Fatalf("Failed to get session history: %v", err)
	}

	if len(history) != 3 {
		t.Errorf("Expected 3 stages, got %d", len(history))
	}

	// Проверяем порядок (должны быть отсортированы по created_at ASC)
	for i := 1; i < len(history); i++ {
		if history[i].CreatedAt.Before(history[i-1].CreatedAt) {
			t.Error("History is not sorted correctly")
		}
	}
}

func TestRevertToStage(t *testing.T) {
	db, err := NewDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create DB: %v", err)
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

	// Создаем сессию
	sessionID, err := db.CreateNormalizationSession(itemID, "Original Name")
	if err != nil {
		t.Fatalf("Failed to create normalization session: %v", err)
	}

	// Создаем несколько стадий
	var stageIDs []int
	for i := 0; i < 3; i++ {
		stage := &NormalizationStage{
			SessionID:       sessionID,
			StageType:       "algorithmic",
			StageName:       "pattern_correction",
			InputName:       "Input",
			OutputName:       "Output",
			AppliedPatterns: "[]",
			Confidence:      0.95,
			Status:          "applied",
		}
		err = db.AddNormalizationStage(stage)
		if err != nil {
			t.Fatalf("Failed to add normalization stage: %v", err)
		}
		// Получаем ID созданной стадии
		var stageID int
		err = db.QueryRow("SELECT id FROM normalization_stages WHERE session_id = ? ORDER BY id DESC LIMIT 1", sessionID).Scan(&stageID)
		if err != nil {
			t.Fatalf("Failed to get stage ID: %v", err)
		}
		stageIDs = append(stageIDs, stageID)
	}

	// Откатываемся ко второй стадии
	targetStageID := stageIDs[1]
	err = db.RevertToStage(sessionID, targetStageID)
	if err != nil {
		t.Fatalf("Failed to revert to stage: %v", err)
	}

	// Проверяем, что осталась только одна стадия
	history, err := db.GetSessionHistory(sessionID)
	if err != nil {
		t.Fatalf("Failed to get session history: %v", err)
	}

	if len(history) != 2 {
		t.Errorf("Expected 2 stages after revert, got %d", len(history))
	}

	// Проверяем, что последняя стадия - это целевая
	if history[len(history)-1].ID != targetStageID {
		t.Errorf("Expected last stage ID %d, got %d", targetStageID, history[len(history)-1].ID)
	}
}

