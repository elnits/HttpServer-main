package main

import (
	"fmt"
	"log"
	"os"

	"httpserver/database"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Использование: show_normalization_summary <путь_к_базе.db>")
		os.Exit(1)
	}

	dbPath := os.Args[1]

	db, err := database.NewDB(dbPath)
	if err != nil {
		log.Fatalf("Ошибка подключения: %v", err)
	}
	defer db.Close()

	fmt.Println("=== Сводка по нормализации и классификации ===")

	// Общая статистика
	var totalItems int
	db.QueryRow("SELECT COUNT(*) FROM catalog_items").Scan(&totalItems)
	fmt.Printf("Всего элементов в справочнике: %d\n", totalItems)

	// Классифицированные
	var classifiedCount int
	db.QueryRow("SELECT COUNT(*) FROM catalog_items WHERE category_level1 IS NOT NULL AND category_level1 != ''").Scan(&classifiedCount)
	fmt.Printf("Классифицировано: %d (%.1f%%)\n", classifiedCount, float64(classifiedCount)/float64(totalItems)*100)

	// Классификаторы
	classifiers, err := db.GetActiveCategoryClassifiers()
	if err == nil {
		fmt.Printf("\nАктивных классификаторов: %d\n", len(classifiers))
		for _, cl := range classifiers {
			fmt.Printf("  - %s (ID: %d, глубина: %d)\n", cl.Name, cl.ID, cl.MaxDepth)
		}
	}

	// Примеры классифицированных элементов
	fmt.Println("\n=== Примеры классифицированных элементов ===")
	query := `
		SELECT id, code, name, category_level1, category_level2, classification_confidence
		FROM catalog_items
		WHERE category_level1 IS NOT NULL AND category_level1 != ''
		LIMIT 10
	`
	rows, err := db.Query(query)
	if err == nil {
		defer rows.Close()
		count := 0
		for rows.Next() && count < 10 {
			var id int
			var code, name, level1, level2 string
			var confidence float64
			if err := rows.Scan(&id, &code, &name, &level1, &level2, &confidence); err == nil {
				count++
				fmt.Printf("\n%d. %s (код: %s)\n", count, name, code)
				fmt.Printf("   Категория: %s", level1)
				if level2 != "" {
					fmt.Printf(" / %s", level2)
				}
				fmt.Printf("\n   Уверенность: %.1f%%\n", confidence*100)
			}
		}
		if count == 0 {
			fmt.Println("Классифицированные элементы не найдены.")
			fmt.Println("\nДля запуска классификации:")
			fmt.Println("  $env:ARLIAI_API_KEY = 'ваш_ключ'")
			fmt.Println("  go run cmd/classify_catalog_items/main.go 1c_data.db 1 top_priority")
		}
	}

	// Статистика по категориям
	if classifiedCount > 0 {
		fmt.Println("\n=== Статистика по категориям (топ-10) ===")
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
					fmt.Printf("  %s: %d элементов\n", category, count)
				}
			}
		}
	}

	fmt.Println()
}

