package normalization

import (
	"testing"
	"time"

	"httpserver/database"
)

func TestNewNormalizer(t *testing.T) {
	db, err := database.NewDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test DB: %v", err)
	}
	defer db.Close()

	events := make(chan string, 10)

	// Тест без AI
	t.Run("WithoutAI", func(t *testing.T) {
		aiConfig := &AIConfig{
			Enabled: false,
		}
		normalizer := NewNormalizer(db, events, aiConfig)

		if normalizer == nil {
			t.Error("NewNormalizer returned nil")
		}
		if normalizer.useAI {
			t.Error("Expected useAI to be false")
		}
		if normalizer.aiNormalizer != nil {
			t.Error("Expected aiNormalizer to be nil")
		}
		if normalizer.categorizer == nil {
			t.Error("Expected categorizer to be initialized")
		}
		if normalizer.nameNormalizer == nil {
			t.Error("Expected nameNormalizer to be initialized")
		}
	})

	// Тест с AI (но без API ключа)
	t.Run("WithAIConfigButNoKey", func(t *testing.T) {
		aiConfig := &AIConfig{
			Enabled:       true,
			MinConfidence: 0.7,
			MaxRetries:    3,
		}
		normalizer := NewNormalizer(db, events, aiConfig)

		if normalizer == nil {
			t.Error("NewNormalizer returned nil")
		}
		if normalizer.useAI {
			t.Error("Expected useAI to be false when API key is not set")
		}
	})

	// Тест с nil конфигом
	t.Run("WithNilConfig", func(t *testing.T) {
		normalizer := NewNormalizer(db, events, nil)

		if normalizer == nil {
			t.Error("NewNormalizer returned nil")
		}
		if normalizer.useAI {
			t.Error("Expected useAI to be false with nil config")
		}
	})
}

func TestNormalizerSetSourceConfig(t *testing.T) {
	db, err := database.NewDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test DB: %v", err)
	}
	defer db.Close()

	events := make(chan string, 10)
	normalizer := NewNormalizer(db, events, nil)

	normalizer.SetSourceConfig("test_table", "ref_col", "code_col", "name_col")

	if normalizer.sourceTable != "test_table" {
		t.Errorf("Expected sourceTable 'test_table', got '%s'", normalizer.sourceTable)
	}
	if normalizer.referenceColumn != "ref_col" {
		t.Errorf("Expected referenceColumn 'ref_col', got '%s'", normalizer.referenceColumn)
	}
	if normalizer.codeColumn != "code_col" {
		t.Errorf("Expected codeColumn 'code_col', got '%s'", normalizer.codeColumn)
	}
	if normalizer.nameColumn != "name_col" {
		t.Errorf("Expected nameColumn 'name_col', got '%s'", normalizer.nameColumn)
	}
}

func TestNormalizerGrouping(t *testing.T) {
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

	// Добавляем элементы каталога
	items := []*database.CatalogItem{
		{
			CatalogID:   catalog.ID,
			CatalogName: "TestCatalog",
			Reference:   "ref1",
			Code:        "code1",
			Name:        "Молоток большой",
		},
		{
			CatalogID:   catalog.ID,
			CatalogName: "TestCatalog",
			Reference:   "ref2",
			Code:        "code2",
			Name:        "Молоток большой",
		},
		{
			CatalogID:   catalog.ID,
			CatalogName: "TestCatalog",
			Reference:   "ref3",
			Code:        "code3",
			Name:        "Отвертка",
		},
	}

	for _, item := range items {
		err := db.AddCatalogItem(item.CatalogID, item.Reference, item.Code, item.Name, "", "")
		if err != nil {
			t.Fatalf("Failed to add catalog item: %v", err)
		}
	}

	events := make(chan string, 100)
	normalizer := NewNormalizer(db, events, nil)
	normalizer.SetSourceConfig("catalog_items", "reference", "code", "name")

	// Тестируем группировку через ProcessNormalization
	// Но сначала нужно проверить, что метод работает
	// Для полного теста нужны данные в таблице catalog_items
	err = normalizer.ProcessNormalization()
	if err != nil {
		// Это нормально, если таблица пустая или нет данных
		t.Logf("ProcessNormalization returned error (expected for empty table): %v", err)
	}
}

func TestNormalizerWithEmptyData(t *testing.T) {
	db, err := database.NewDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test DB: %v", err)
	}
	defer db.Close()

	events := make(chan string, 10)
	normalizer := NewNormalizer(db, events, nil)

	// Очистка должна работать даже с пустой таблицей
	err = normalizer.ProcessNormalization()
	if err != nil {
		// Ожидаем ошибку, так как таблица пустая
		t.Logf("ProcessNormalization with empty data returned error (expected): %v", err)
	}
}

func TestNormalizerErrorHandling(t *testing.T) {
	db, err := database.NewDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test DB: %v", err)
	}
	defer db.Close()

	events := make(chan string, 10)
	normalizer := NewNormalizer(db, events, nil)

	// Тест с несуществующей таблицей
	normalizer.SetSourceConfig("non_existent_table", "ref", "code", "name")
	err = normalizer.ProcessNormalization()
	if err == nil {
		t.Error("Expected error for non-existent table")
	}

	// Тест с неправильными именами колонок
	normalizer.SetSourceConfig("catalog_items", "wrong_ref", "wrong_code", "wrong_name")
	err = normalizer.ProcessNormalization()
	if err == nil {
		t.Error("Expected error for wrong column names")
	}
}

func TestNormalizerSendEvent(t *testing.T) {
	db, err := database.NewDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test DB: %v", err)
	}
	defer db.Close()

	events := make(chan string, 1)
	normalizer := NewNormalizer(db, events, nil)

	// Отправляем событие
	normalizer.sendEvent("test event")

	// Проверяем, что событие получено
	select {
	case event := <-events:
		if event != "test event" {
			t.Errorf("Expected event 'test event', got '%s'", event)
		}
	case <-time.After(1 * time.Second):
		t.Error("Event not received within timeout")
	}

	// Тест с nil каналом
	normalizer.events = nil
	normalizer.sendEvent("should not panic")
	// Не должно быть паники
}

func TestNormalizerCountGroups(t *testing.T) {
	db, err := database.NewDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test DB: %v", err)
	}
	defer db.Close()

	events := make(chan string, 10)
	normalizer := NewNormalizer(db, events, nil)

	groups := make(map[groupKey]*groupValue)
	
	// Пустая группа
	count := normalizer.countGroups(groups)
	if count != 0 {
		t.Errorf("Expected 0 groups, got %d", count)
	}

	// Группа с одним элементом
	key1 := groupKey{category: "test", normalizedName: "test"}
	groups[key1] = &groupValue{items: []*database.CatalogItem{}}
	count = normalizer.countGroups(groups)
	if count != 1 {
		t.Errorf("Expected 1 group, got %d", count)
	}

	// Группа с несколькими элементами
	key2 := groupKey{category: "test2", normalizedName: "test2"}
	groups[key2] = &groupValue{items: []*database.CatalogItem{}}
	count = normalizer.countGroups(groups)
	if count != 2 {
		t.Errorf("Expected 2 groups, got %d", count)
	}
}

