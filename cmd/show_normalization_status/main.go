package main

import (
	"fmt"
	"log"
	"os"

	"httpserver/database"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Использование: show_normalization_status <путь_к_базе.db>")
		os.Exit(1)
	}

	dbPath := os.Args[1]

	db, err := database.NewDB(dbPath)
	if err != nil {
		log.Fatalf("Ошибка подключения: %v", err)
	}
	defer db.Close()

	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║     СТАТУС НОРМАЛИЗАЦИИ И КЛАССИФИКАЦИИ НСИ                 ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	fmt.Println()

	// Общая статистика
	var totalItems int
	db.QueryRow("SELECT COUNT(*) FROM catalog_items").Scan(&totalItems)
	fmt.Printf("📊 Всего элементов в справочнике: %d\n", totalItems)

	// Классифицированные
	var classifiedCount int
	err = db.QueryRow("SELECT COUNT(*) FROM catalog_items WHERE category_level1 IS NOT NULL AND category_level1 != ''").Scan(&classifiedCount)
	if err != nil {
		classifiedCount = 0
	}
	
	percentage := 0.0
	if totalItems > 0 {
		percentage = float64(classifiedCount) / float64(totalItems) * 100
	}
	
	fmt.Printf("✅ Классифицировано: %d (%.1f%%)\n", classifiedCount, percentage)
	fmt.Printf("⏳ Осталось классифицировать: %d\n", totalItems-classifiedCount)
	fmt.Println()

	// Классификаторы
	fmt.Println("📋 Классификаторы:")
	classifiers, err := db.GetActiveCategoryClassifiers()
	if err == nil && len(classifiers) > 0 {
		for _, cl := range classifiers {
			fmt.Printf("   ✓ %s (ID: %d, глубина: %d уровней)\n", cl.Name, cl.ID, cl.MaxDepth)
		}
	} else {
		fmt.Println("   ⚠ Классификаторы не найдены")
		fmt.Println("   Для загрузки КПВЭД:")
		fmt.Println("     go run cmd/load_kpved/main.go КПВЭД.txt 1c_data.db")
	}
	fmt.Println()

	// Примеры элементов
	fmt.Println("📦 Примеры элементов справочника:")
	query := `
		SELECT id, code, name
		FROM catalog_items
		WHERE name IS NOT NULL AND name != ''
		ORDER BY id
		LIMIT 10
	`
	rows, err := db.Query(query)
	if err == nil {
		defer rows.Close()
		count := 0
		for rows.Next() && count < 10 {
			var id int
			var code, name string
			if err := rows.Scan(&id, &code, &name); err == nil {
				count++
				fmt.Printf("   %d. [%s] %s\n", count, code, name)
			}
		}
	}
	fmt.Println()

	// Примеры классифицированных (если есть)
	if classifiedCount > 0 {
		fmt.Println("✅ Примеры классифицированных элементов:")
		classifiedQuery := `
			SELECT id, code, name, category_level1, category_level2, classification_confidence
			FROM catalog_items
			WHERE category_level1 IS NOT NULL AND category_level1 != ''
			LIMIT 5
		`
		classifiedRows, err := db.Query(classifiedQuery)
		if err == nil {
			defer classifiedRows.Close()
			count := 0
			for classifiedRows.Next() && count < 5 {
				var id int
				var code, name, level1, level2 string
				var confidence float64
				if err := classifiedRows.Scan(&id, &code, &name, &level1, &level2, &confidence); err == nil {
					count++
					fmt.Printf("\n   Пример #%d:\n", count)
					fmt.Printf("      Название: %s\n", name)
					fmt.Printf("      Категория: %s", level1)
					if level2 != "" {
						fmt.Printf(" / %s", level2)
					}
					fmt.Printf("\n      Уверенность: %.1f%%\n", confidence*100)
				}
			}
		}
		fmt.Println()

		// Статистика по категориям
		fmt.Println("📈 Топ-10 категорий уровня 1:")
		statsQuery := `
			SELECT category_level1, COUNT(*) as count 
			FROM catalog_items 
			WHERE category_level1 IS NOT NULL AND category_level1 != ''
			GROUP BY category_level1 
			ORDER BY count DESC
			LIMIT 10
		`
		statsRows, err := db.Query(statsQuery)
		if err == nil {
			defer statsRows.Close()
			for statsRows.Next() {
				var category string
				var count int
				if err := statsRows.Scan(&category, &count); err == nil {
					fmt.Printf("   • %s: %d элементов\n", category, count)
				}
			}
		}
		fmt.Println()
	} else {
		fmt.Println("⚠ Классифицированные элементы не найдены")
		fmt.Println()
		fmt.Println("📝 Для запуска классификации:")
		fmt.Println("   1. Установите API ключ:")
		fmt.Println("      $env:ARLIAI_API_KEY = '597dbe7e-16ca-4803-ab17-5fa084909f37'")
		fmt.Println()
		fmt.Println("   2. Запустите классификацию (с лимитом для теста):")
		fmt.Println("      go run cmd/classify_catalog_items/main.go 1c_data.db 1 top_priority 10")
		fmt.Println()
		fmt.Println("   3. Для полной классификации всех 15973 элементов:")
		fmt.Println("      go run cmd/classify_catalog_items/main.go 1c_data.db 1 top_priority")
		fmt.Println()
		fmt.Println("   ⚠ Примечание: Полная классификация может занять несколько часов")
		fmt.Println()
	}

	// Проверяем наличие нормализованных имен
	var normalizedCount int
	db.QueryRow("SELECT COUNT(*) FROM catalog_items WHERE normalized_name IS NOT NULL AND normalized_name != ''").Scan(&normalizedCount)
	if normalizedCount > 0 {
		fmt.Printf("✨ Нормализованных названий: %d\n", normalizedCount)
		fmt.Println()
		fmt.Println("📝 Примеры нормализованных названий:")
		normQuery := `
			SELECT name, normalized_name
			FROM catalog_items
			WHERE normalized_name IS NOT NULL AND normalized_name != '' AND normalized_name != name
			LIMIT 5
		`
		normRows, err := db.Query(normQuery)
		if err == nil {
			defer normRows.Close()
			for normRows.Next() {
				var original, normalized string
				if err := normRows.Scan(&original, &normalized); err == nil {
					fmt.Printf("   • %s → %s\n", original, normalized)
				}
			}
		}
		fmt.Println()
	}

	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║                    ТЕКУЩИЙ СТАТУС                            ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Printf("📦 Элементов в базе: %d\n", totalItems)
	fmt.Printf("✅ Классифицировано: %d (%.1f%%)\n", classifiedCount, percentage)
	fmt.Printf("📋 Классификаторов: %d\n", len(classifiers))
	fmt.Println()
}

