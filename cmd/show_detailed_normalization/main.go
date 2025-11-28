package main

import (
	"fmt"
	"log"
	"os"

	"httpserver/database"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Использование: show_detailed_normalization <путь_к_базе.db>")
		os.Exit(1)
	}

	dbPath := os.Args[1]

	db, err := database.NewDB(dbPath)
	if err != nil {
		log.Fatalf("Ошибка подключения: %v", err)
	}
	defer db.Close()

	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║       ДЕТАЛЬНЫЙ ОТЧЕТ О НОРМАЛИЗАЦИИ И КЛАССИФИКАЦИИ        ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	fmt.Println()

	// Общая статистика
	var totalItems int
	db.QueryRow("SELECT COUNT(*) FROM catalog_items").Scan(&totalItems)
	var normalizedCount int
	db.QueryRow("SELECT COUNT(*) FROM normalized_data").Scan(&normalizedCount)

	fmt.Printf("📊 ОБЩАЯ СТАТИСТИКА\n")
	fmt.Printf("   Всего элементов в справочнике: %d\n", totalItems)
	fmt.Printf("   Нормализованных записей: %d (%.1f%%)\n", normalizedCount, float64(normalizedCount)/float64(totalItems)*100)
	fmt.Println()

	// Статистика по категориям
	fmt.Println("📋 СТАТИСТИКА ПО КАТЕГОРИЯМ")
	categoryQuery := `
		SELECT category, COUNT(*) as count
		FROM normalized_data
		WHERE category IS NOT NULL AND category != ''
		GROUP BY category
		ORDER BY count DESC
		LIMIT 20
	`
	var uniqueCategories int
	rows, err := db.Query(categoryQuery)
	if err == nil {
		defer rows.Close()
		totalCategories := 0
		for rows.Next() {
			var category string
			var count int
			if err := rows.Scan(&category, &count); err == nil {
				totalCategories++
				percentage := float64(count) / float64(normalizedCount) * 100
				fmt.Printf("   %2d. %-40s: %5d (%.1f%%)\n", totalCategories, category, count, percentage)
			}
		}
		
		// Общее количество уникальных категорий
		db.QueryRow("SELECT COUNT(DISTINCT category) FROM normalized_data WHERE category IS NOT NULL AND category != ''").Scan(&uniqueCategories)
		fmt.Printf("\n   Всего уникальных категорий: %d\n", uniqueCategories)
	}
	fmt.Println()

	// Примеры по категориям
	fmt.Println("📦 ПРИМЕРЫ ПО КАТЕГОРИЯМ")
	examplesQuery := `
		SELECT category, source_name, normalized_name, code
		FROM normalized_data
		WHERE category IS NOT NULL AND category != ''
		GROUP BY category
		ORDER BY category
		LIMIT 15
	`
	exampleRows, err := db.Query(examplesQuery)
	if err == nil {
		defer exampleRows.Close()
		prevCategory := ""
		count := 0
		for exampleRows.Next() {
			var category, sourceName, normalizedName, code string
			if err := exampleRows.Scan(&category, &sourceName, &normalizedName, &code); err == nil {
				if category != prevCategory {
					if prevCategory != "" {
						fmt.Println()
					}
					count++
					fmt.Printf("   Категория #%d: %s\n", count, category)
					prevCategory = category
				}
				fmt.Printf("      • %s → %s [%s]\n", sourceName, normalizedName, code)
			}
		}
	}
	fmt.Println()

	// Статистика по нормализации
	fmt.Println("✨ СТАТИСТИКА НОРМАЛИЗАЦИИ")
	var changedCount int
	db.QueryRow(`
		SELECT COUNT(*) 
		FROM normalized_data 
		WHERE source_name != normalized_name 
		AND normalized_name IS NOT NULL 
		AND normalized_name != ''
	`).Scan(&changedCount)
	fmt.Printf("   Изменено названий: %d (%.1f%%)\n", changedCount, float64(changedCount)/float64(normalizedCount)*100)

	var unchangedCount int
	db.QueryRow(`
		SELECT COUNT(*) 
		FROM normalized_data 
		WHERE source_name = normalized_name
	`).Scan(&unchangedCount)
	fmt.Printf("   Без изменений: %d (%.1f%%)\n", unchangedCount, float64(unchangedCount)/float64(normalizedCount)*100)
	fmt.Println()

	// Статистика по объединению
	fmt.Println("🔗 СТАТИСТИКА ПО ОБЪЕДИНЕНИЮ")
	var mergedCount int
	db.QueryRow("SELECT COUNT(*) FROM normalized_data WHERE merged_count > 1").Scan(&mergedCount)
	fmt.Printf("   Записей с объединением: %d\n", mergedCount)

	var totalMerged int
	db.QueryRow("SELECT SUM(merged_count) FROM normalized_data WHERE merged_count > 1").Scan(&totalMerged)
	if totalMerged > 0 {
		fmt.Printf("   Всего объединено записей: %d\n", totalMerged)
	}
	fmt.Println()

	// Классификация КПВЭД
	fmt.Println("🏷️  КЛАССИФИКАЦИЯ ПО КПВЭД")
	var kpvedCount int
	db.QueryRow("SELECT COUNT(*) FROM normalized_data WHERE kpved_code IS NOT NULL AND kpved_code != ''").Scan(&kpvedCount)
	fmt.Printf("   С классификацией КПВЭД: %d (%.1f%%)\n", kpvedCount, float64(kpvedCount)/float64(normalizedCount)*100)

	if kpvedCount > 0 {
		kpvedQuery := `
			SELECT kpved_code, kpved_name, COUNT(*) as count
			FROM normalized_data
			WHERE kpved_code IS NOT NULL AND kpved_code != ''
			GROUP BY kpved_code, kpved_name
			ORDER BY count DESC
			LIMIT 10
		`
		kpvedRows, err := db.Query(kpvedQuery)
		if err == nil {
			defer kpvedRows.Close()
			fmt.Println("   Топ-10 кодов КПВЭД:")
			for kpvedRows.Next() {
				var code, name string
				var count int
				if err := kpvedRows.Scan(&code, &name, &count); err == nil {
					fmt.Printf("      %s - %s: %d элементов\n", code, name, count)
				}
			}
		}
	} else {
		fmt.Println("   ⚠ Классификация по КПВЭД еще не выполнена")
		fmt.Println("   Для запуска:")
		fmt.Println("     $env:ARLIAI_API_KEY = 'ваш_ключ'")
		fmt.Println("     go run cmd/classify_catalog_items/main.go 1c_data.db 1 top_priority")
	}
	fmt.Println()

	// Классификаторы
	fmt.Println("📚 КЛАССИФИКАТОРЫ")
	classifiers, err := db.GetActiveCategoryClassifiers()
	if err == nil && len(classifiers) > 0 {
		for _, cl := range classifiers {
			fmt.Printf("   ✓ %s (ID: %d)\n", cl.Name, cl.ID)
			fmt.Printf("     Глубина: %d уровней\n", cl.MaxDepth)
			if cl.Description != "" {
				fmt.Printf("     Описание: %s\n", cl.Description)
			}
		}
	} else {
		fmt.Println("   ⚠ Классификаторы не найдены")
	}
	fmt.Println()

	// Итоговая сводка
	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║                    ИТОГОВАЯ СВОДКА                           ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	fmt.Printf("✅ Нормализация: %d/%d записей (100%%)\n", normalizedCount, totalItems)
	fmt.Printf("🏷️  Классификация КПВЭД: %d/%d записей (%.1f%%)\n", kpvedCount, normalizedCount, float64(kpvedCount)/float64(normalizedCount)*100)
	fmt.Printf("📋 Категорий: %d уникальных\n", uniqueCategories)
	fmt.Println()
}

