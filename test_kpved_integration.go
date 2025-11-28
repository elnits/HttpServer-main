package main

import (
	"fmt"
	"log"
	"os"

	"httpserver/database"
	"httpserver/nomenclature"
	"httpserver/normalization"

	_ "github.com/mattn/go-sqlite3"
)

func main() {
	// Проверяем API ключ
	apiKey := os.Getenv("ARLIAI_API_KEY")
	if apiKey == "" {
		log.Fatal("ARLIAI_API_KEY не установлен. Установите переменную окружения.")
	}

	// Открываем БД
	db, err := database.NewDB("data/normalized_data.db")
	if err != nil {
		log.Fatalf("Ошибка открытия БД: %v", err)
	}
	defer db.Close()

	fmt.Println("=== ТЕСТ КПВЭД ИНТЕГРАЦИИ ===\n")

	// Проверяем, загружен ли классификатор
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM kpved_classifier").Scan(&count)
	if err != nil || count == 0 {
		log.Fatal("КПВЭД классификатор не загружен. Запустите: go run cmd/load_kpved/main.go")
	}
	fmt.Printf("✓ КПВЭД классификатор загружен: %d записей\n", count)

	// Создаем AI клиент
	model := os.Getenv("ARLIAI_MODEL")
	if model == "" {
		model = "GLM-4.5-Air"
	}
	aiClient := nomenclature.NewAIClient(apiKey, model)
	fmt.Printf("✓ AI клиент создан (модель: %s)\n", model)

	// Создаем HierarchicalClassifier
	classifier, err := normalization.NewHierarchicalClassifier(db, aiClient)
	if err != nil {
		log.Fatalf("Ошибка создания классификатора: %v", err)
	}
	fmt.Println("✓ HierarchicalClassifier инициализирован")

	// Добавляем тестовые записи
	testItems := []struct {
		name     string
		category string
	}{
		{"Болт М8х20", "Крепеж"},
		{"Гайка М8", "Крепеж"},
		{"Краска масляная белая", "Лакокрасочные материалы"},
	}

	fmt.Println("\nДобавление тестовых записей в normalized_data:")
	for _, item := range testItems {
		_, err := db.Exec(`
			INSERT OR REPLACE INTO normalized_data
			(normalized_name, normalized_reference, category, merged_count, source_name, source_reference, code)
			VALUES (?, ?, ?, 1, ?, ?, ?)
		`, item.name, item.name, item.category, item.name, item.name, "TEST")
		if err != nil {
			log.Printf("Ошибка добавления записи '%s': %v", item.name, err)
			continue
		}
		fmt.Printf("  ✓ %s (%s)\n", item.name, item.category)
	}

	// Получаем ID добавленных записей
	rows, err := db.Query(`
		SELECT id, normalized_name, category
		FROM normalized_data
		WHERE code = 'TEST' AND (kpved_code IS NULL OR kpved_code = '')
	`)
	if err != nil {
		log.Fatalf("Ошибка получения тестовых записей: %v", err)
	}
	defer rows.Close()

	type TestRecord struct {
		ID             int
		NormalizedName string
		Category       string
	}

	var records []TestRecord
	for rows.Next() {
		var r TestRecord
		if err := rows.Scan(&r.ID, &r.NormalizedName, &r.Category); err != nil {
			log.Printf("Ошибка чтения записи: %v", err)
			continue
		}
		records = append(records, r)
	}

	fmt.Printf("\nНайдено %d тестовых записей для классификации\n", len(records))

	if len(records) == 0 {
		fmt.Println("Нет записей для тестирования. Завершение.")
		return
	}

	// Классифицируем записи
	fmt.Println("\nКлассификация по КПВЭД:")
	for i, record := range records {
		fmt.Printf("\n[%d/%d] %s\n", i+1, len(records), record.NormalizedName)

		result, err := classifier.Classify(record.NormalizedName, record.Category)
		if err != nil {
			fmt.Printf("  ✗ Ошибка классификации: %v\n", err)
			continue
		}

		fmt.Printf("  ✓ Код КПВЭД: %s\n", result.FinalCode)
		fmt.Printf("  ✓ Название: %s\n", result.FinalName)
		fmt.Printf("  ✓ Уверенность: %.1f%%\n", result.FinalConfidence*100)
		fmt.Printf("  ✓ Шагов: %d, AI вызовов: %d, Время: %dмс\n",
			len(result.Steps), result.AICallsCount, result.TotalDuration)

		// Обновляем запись в БД
		_, err = db.Exec(`
			UPDATE normalized_data
			SET kpved_code = ?, kpved_name = ?, kpved_confidence = ?
			WHERE id = ?
		`, result.FinalCode, result.FinalName, result.FinalConfidence, record.ID)

		if err != nil {
			fmt.Printf("  ✗ Ошибка обновления БД: %v\n", err)
		} else {
			fmt.Println("  ✓ Запись обновлена в БД")
		}
	}

	// Проверяем результаты
	fmt.Println("\n=== РЕЗУЛЬТАТЫ ===")
	rows2, err := db.Query(`
		SELECT normalized_name, category, kpved_code, kpved_name, kpved_confidence
		FROM normalized_data
		WHERE code = 'TEST'
	`)
	if err != nil {
		log.Fatalf("Ошибка получения результатов: %v", err)
	}
	defer rows2.Close()

	for rows2.Next() {
		var name, category, kpvedCode, kpvedName string
		var confidence float64
		if err := rows2.Scan(&name, &category, &kpvedCode, &kpvedName, &confidence); err != nil {
			log.Printf("Ошибка чтения результата: %v", err)
			continue
		}
		fmt.Printf("\n%s (%s)\n", name, category)
		fmt.Printf("  КПВЭД: %s - %s\n", kpvedCode, kpvedName)
		fmt.Printf("  Уверенность: %.1f%%\n", confidence*100)
	}

	fmt.Println("\n✓ Тест завершен успешно!")
}
