package main

import (
	"fmt"
	"log"
	"os"

	"httpserver/database"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Использование: show_normalized_results <путь_к_базе.db> [limit]")
		os.Exit(1)
	}

	dbPath := os.Args[1]
	limit := 20
	if len(os.Args) >= 3 {
		fmt.Sscanf(os.Args[2], "%d", &limit)
	}

	db, err := database.NewDB(dbPath)
	if err != nil {
		log.Fatalf("Ошибка подключения: %v", err)
	}
	defer db.Close()

	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║          РЕЗУЛЬТАТЫ НОРМАЛИЗАЦИИ И КЛАССИФИКАЦИИ             ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	fmt.Println()

	// Проверяем normalized_data
	var normalizedCount int
	err = db.QueryRow("SELECT COUNT(*) FROM normalized_data").Scan(&normalizedCount)
	if err != nil {
		normalizedCount = 0
	}

	if normalizedCount > 0 {
		fmt.Printf("✅ Найдено нормализованных записей: %d\n\n", normalizedCount)

		// Получаем примеры через прямой запрос
		query := `
			SELECT id, source_reference, source_name, code, normalized_name,
			       normalized_reference, category, merged_count, 
			       COALESCE(ai_confidence, 0) as ai_confidence,
			       COALESCE(ai_reasoning, '') as ai_reasoning,
			       COALESCE(processing_level, '') as processing_level,
			       COALESCE(kpved_code, '') as kpved_code,
			       COALESCE(kpved_name, '') as kpved_name,
			       COALESCE(kpved_confidence, 0) as kpved_confidence,
			       created_at
			FROM normalized_data
			ORDER BY id
			LIMIT ?
		`
		rows, err := db.Query(query, limit)
		if err != nil {
			log.Printf("Ошибка получения нормализованных записей: %v", err)
		} else {
			defer rows.Close()
			count := 0
			for rows.Next() {
				var id, mergedCount int
				var sourceRef, sourceName, code, normalizedName, normalizedRef, category string
				var aiConfidence, kpvedConfidence float64
				var aiReasoning, processingLevel, kpvedCode, kpvedName string
				var createdAt string
				
				if err := rows.Scan(&id, &sourceRef, &sourceName, &code, &normalizedName,
					&normalizedRef, &category, &mergedCount, &aiConfidence, &aiReasoning,
					&processingLevel, &kpvedCode, &kpvedName, &kpvedConfidence, &createdAt); err != nil {
					log.Printf("Ошибка сканирования: %v", err)
					continue
				}
				
				count++
				if count == 1 {
					fmt.Printf("📋 Примеры нормализованных записей:\n\n")
				}
				
				fmt.Printf("═══ Пример #%d ═══\n", count)
				fmt.Printf("Исходное название: %s\n", sourceName)
				if normalizedName != "" && normalizedName != sourceName {
					fmt.Printf("Нормализованное:   %s\n", normalizedName)
				}
				if code != "" {
					fmt.Printf("Код:               %s\n", code)
				}
				if category != "" {
					fmt.Printf("Категория:         %s\n", category)
				}
				if kpvedCode != "" {
					fmt.Printf("КПВЭД код:         %s\n", kpvedCode)
				}
				if kpvedName != "" {
					fmt.Printf("КПВЭД название:    %s\n", kpvedName)
				}
				if kpvedConfidence > 0 {
					fmt.Printf("Уверенность КПВЭД: %.1f%%\n", kpvedConfidence*100)
				}
				if aiConfidence > 0 {
					fmt.Printf("Уверенность AI:    %.1f%%\n", aiConfidence*100)
				}
				if mergedCount > 0 {
					fmt.Printf("Объединено записей: %d\n", mergedCount)
				}
				fmt.Println()
			}
			
			if count > 0 {
				// Статистика
				fmt.Println("📊 Статистика:")
				var avgConfidence float64
				db.QueryRow("SELECT AVG(ai_confidence) FROM normalized_data WHERE ai_confidence > 0").Scan(&avgConfidence)
				if avgConfidence > 0 {
					fmt.Printf("   Средняя уверенность AI: %.1f%%\n", avgConfidence*100)
				}

				var kpvedCount int
				db.QueryRow("SELECT COUNT(*) FROM normalized_data WHERE kpved_code IS NOT NULL AND kpved_code != ''").Scan(&kpvedCount)
				fmt.Printf("   С классификацией КПВЭД: %d (%.1f%%)\n", kpvedCount, float64(kpvedCount)/float64(normalizedCount)*100)
			}
		}
	} else {
		fmt.Println("⚠ Нормализованные записи не найдены в таблице normalized_data")
		fmt.Println()
	}

	// Проверяем catalog_items с нормализованными именами
	var catalogNormalizedCount int
	err = db.QueryRow("SELECT COUNT(*) FROM catalog_items WHERE normalized_name IS NOT NULL AND normalized_name != ''").Scan(&catalogNormalizedCount)
	if err != nil {
		catalogNormalizedCount = 0
	}

	if catalogNormalizedCount > 0 {
		fmt.Printf("✅ Нормализованных названий в catalog_items: %d\n\n", catalogNormalizedCount)
		
		// Примеры
		query := `
			SELECT id, code, name, normalized_name, category_level1, category_level2
			FROM catalog_items
			WHERE normalized_name IS NOT NULL AND normalized_name != '' AND normalized_name != name
			LIMIT ?
		`
		rows, err := db.Query(query, limit)
		if err == nil {
			defer rows.Close()
			fmt.Println("📝 Примеры нормализации названий:")
			count := 0
			for rows.Next() && count < limit {
				var id int
				var code, name, normalized, level1, level2 string
				if err := rows.Scan(&id, &code, &name, &normalized, &level1, &level2); err == nil {
					count++
					fmt.Printf("\n   %d. [%s]\n", count, code)
					fmt.Printf("      Исходное:     %s\n", name)
					fmt.Printf("      Нормализованное: %s\n", normalized)
					if level1 != "" {
						fmt.Printf("      Категория:    %s", level1)
						if level2 != "" {
							fmt.Printf(" / %s", level2)
						}
						fmt.Println()
					}
				}
			}
		}
		fmt.Println()
	}

	// Итоговая сводка
	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║                    ИТОГОВАЯ СВОДКА                           ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	
	var totalCatalogItems int
	db.QueryRow("SELECT COUNT(*) FROM catalog_items").Scan(&totalCatalogItems)
	
	var classifiedCount int
	db.QueryRow("SELECT COUNT(*) FROM catalog_items WHERE category_level1 IS NOT NULL AND category_level1 != ''").Scan(&classifiedCount)
	
	fmt.Printf("📦 Всего элементов в catalog_items: %d\n", totalCatalogItems)
	fmt.Printf("✅ Классифицировано: %d (%.1f%%)\n", classifiedCount, float64(classifiedCount)/float64(totalCatalogItems)*100)
	fmt.Printf("📋 Нормализованных записей: %d\n", normalizedCount)
	fmt.Printf("✨ Нормализованных названий: %d\n", catalogNormalizedCount)
	fmt.Println()
}

