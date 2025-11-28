package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"httpserver/database"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Использование: show_classification_results <путь_к_базе.db> [limit]")
		fmt.Println("Пример: show_classification_results 1c_data.db")
		fmt.Println("Пример: show_classification_results 1c_data.db 50")
		os.Exit(1)
	}

	dbPath := os.Args[1]
	limit := 50
	if len(os.Args) >= 3 {
		fmt.Sscanf(os.Args[2], "%d", &limit)
	}

	db, err := database.NewDB(dbPath)
	if err != nil {
		log.Fatalf("Ошибка подключения: %v", err)
	}
	defer db.Close()

	fmt.Println("=== Результаты классификации элементов справочника ===")

	// Проверяем классифицированные элементы
	query := `
		SELECT id, code, name, category_original, category_level1, category_level2, 
		       classification_strategy, classification_confidence
		FROM catalog_items
		WHERE category_level1 IS NOT NULL AND category_level1 != ''
		ORDER BY id
		LIMIT ?
	`

	rows, err := db.Query(query, limit)
	if err != nil {
		// Возможно, поля еще не созданы
		fmt.Println("Классифицированные элементы не найдены.")
		fmt.Println("Возможные причины:")
		fmt.Println("  1. Поля категорий еще не созданы в таблице catalog_items")
		fmt.Println("  2. Классификация еще не была выполнена")
		fmt.Println("\nДля запуска классификации используйте:")
		fmt.Println("  go run cmd/classify_catalog_items/main.go 1c_data.db 1 top_priority")
		os.Exit(0)
	}
	defer rows.Close()

	var items []struct {
		ID                    int
		Code                  string
		Name                  string
		CategoryOriginal      string
		CategoryLevel1        string
		CategoryLevel2        string
		Strategy              string
		Confidence            float64
	}

	for rows.Next() {
		var item struct {
			ID                    int
			Code                  string
			Name                  string
			CategoryOriginal      string
			CategoryLevel1        string
			CategoryLevel2        string
			Strategy              string
			Confidence            float64
		}
		if err := rows.Scan(&item.ID, &item.Code, &item.Name, &item.CategoryOriginal,
			&item.CategoryLevel1, &item.CategoryLevel2, &item.Strategy, &item.Confidence); err != nil {
			log.Printf("Ошибка сканирования: %v", err)
			continue
		}
		items = append(items, item)
	}

	if len(items) == 0 {
		fmt.Println("Классифицированные элементы не найдены.")
		fmt.Println("\nДля запуска классификации используйте:")
		fmt.Println("  go run cmd/classify_catalog_items/main.go 1c_data.db 1 top_priority")
		os.Exit(0)
	}

	fmt.Printf("Найдено классифицированных элементов: %d\n\n", len(items))

	// Показываем примеры
	for i, item := range items {
		fmt.Printf("=== Пример #%d ===\n", i+1)
		fmt.Printf("ID: %d\n", item.ID)
		fmt.Printf("Код: %s\n", item.Code)
		fmt.Printf("Название: %s\n", item.Name)

		// Парсим оригинальный путь
		var originalPath []string
		if item.CategoryOriginal != "" {
			if err := json.Unmarshal([]byte(item.CategoryOriginal), &originalPath); err == nil {
				fmt.Printf("Полный путь категории: %s\n", formatPath(originalPath))
			} else {
				fmt.Printf("Полный путь категории: %s\n", item.CategoryOriginal)
			}
		}

		fmt.Printf("Категория уровень 1: %s\n", item.CategoryLevel1)
		if item.CategoryLevel2 != "" {
			fmt.Printf("Категория уровень 2: %s\n", item.CategoryLevel2)
		}
		fmt.Printf("Стратегия: %s\n", item.Strategy)
		fmt.Printf("Уверенность: %.2f%%\n", item.Confidence*100)
		fmt.Println()
	}

	// Статистика
	fmt.Println("=== Статистика ===")
	
	// Общее количество классифицированных
	var totalClassified int
	db.QueryRow("SELECT COUNT(*) FROM catalog_items WHERE category_level1 IS NOT NULL AND category_level1 != ''").Scan(&totalClassified)
	fmt.Printf("Всего классифицировано: %d\n", totalClassified)

	// Статистика по категориям уровня 1
	fmt.Println("\nТоп-10 категорий уровня 1:")
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

	// Средняя уверенность
	var avgConfidence float64
	db.QueryRow("SELECT AVG(classification_confidence) FROM catalog_items WHERE classification_confidence > 0").Scan(&avgConfidence)
	fmt.Printf("\nСредняя уверенность классификации: %.2f%%\n", avgConfidence*100)
}

func formatPath(path []string) string {
	if len(path) == 0 {
		return ""
	}
	result := path[0]
	for i := 1; i < len(path); i++ {
		result += " / " + path[i]
	}
	return result
}

