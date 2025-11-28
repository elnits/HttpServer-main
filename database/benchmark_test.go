package database

import (
	"testing"
)

// BenchmarkInsertCatalogItem тестирует производительность вставки элементов каталога
func BenchmarkInsertCatalogItem(b *testing.B) {
	db, err := NewDB(":memory:")
	if err != nil {
		b.Fatalf("Failed to create test DB: %v", err)
	}
	defer db.Close()

	upload, err := db.CreateUpload("test-uuid", "8.3", "test-config")
	if err != nil {
		b.Fatalf("Failed to create upload: %v", err)
	}

	catalog, err := db.AddCatalog(upload.ID, "TestCatalog", "test_catalog")
	if err != nil {
		b.Fatalf("Failed to create catalog: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		err := db.AddCatalogItem(catalog.ID, "ref", "code", "Test Item", "", "")
		if err != nil {
			b.Fatalf("Failed to insert item: %v", err)
		}
	}
}

// BenchmarkGetCatalogItems тестирует производительность получения элементов каталога
func BenchmarkGetCatalogItems(b *testing.B) {
	db, err := NewDB(":memory:")
	if err != nil {
		b.Fatalf("Failed to create test DB: %v", err)
	}
	defer db.Close()

	upload, err := db.CreateUpload("test-uuid", "8.3", "test-config")
	if err != nil {
		b.Fatalf("Failed to create upload: %v", err)
	}

	catalog, err := db.AddCatalog(upload.ID, "TestCatalog", "test_catalog")
	if err != nil {
		b.Fatalf("Failed to create catalog: %v", err)
	}

	// Добавляем тестовые данные
	for i := 0; i < 100; i++ {
		err := db.AddCatalogItem(catalog.ID, "ref", "code", "Test Item", "", "")
		if err != nil {
			b.Fatalf("Failed to insert item: %v", err)
		}
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := db.GetCatalogItemsByUpload(upload.ID, []string{"TestCatalog"}, 0, 10)
		if err != nil {
			b.Fatalf("Failed to get items: %v", err)
		}
	}
}

// BenchmarkQueryRow тестирует производительность простых запросов
func BenchmarkQueryRow(b *testing.B) {
	db, err := NewDB(":memory:")
	if err != nil {
		b.Fatalf("Failed to create test DB: %v", err)
	}
	defer db.Close()

	upload, err := db.CreateUpload("test-uuid", "8.3", "test-config")
	if err != nil {
		b.Fatalf("Failed to create upload: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var count int
		err := db.QueryRow("SELECT COUNT(*) FROM uploads WHERE id = ?", upload.ID).Scan(&count)
		if err != nil {
			b.Fatalf("Failed to query: %v", err)
		}
	}
}

// BenchmarkCreateNormalizationSession тестирует производительность создания сессий
func BenchmarkCreateNormalizationSession(b *testing.B) {
	db, err := NewDB(":memory:")
	if err != nil {
		b.Fatalf("Failed to create test DB: %v", err)
	}
	defer db.Close()

	upload, err := db.CreateUpload("test-uuid", "8.3", "test-config")
	if err != nil {
		b.Fatalf("Failed to create upload: %v", err)
	}

	catalog, err := db.AddCatalog(upload.ID, "TestCatalog", "test_catalog")
	if err != nil {
		b.Fatalf("Failed to create catalog: %v", err)
	}

	err = db.AddCatalogItem(catalog.ID, "ref1", "code1", "Test Item", "", "")
	if err != nil {
		b.Fatalf("Failed to add catalog item: %v", err)
	}

	var itemID int
	err = db.QueryRow("SELECT id FROM catalog_items WHERE reference = ?", "ref1").Scan(&itemID)
	if err != nil {
		b.Fatalf("Failed to get item ID: %v", err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := db.CreateNormalizationSession(itemID, "Test Item")
		if err != nil {
			b.Fatalf("Failed to create session: %v", err)
		}
	}
}

