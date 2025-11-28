package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"

	_ "github.com/mattn/go-sqlite3"
	"httpserver/database"
	"httpserver/normalization"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Использование: test_patterns <путь_к_базе> [limit]")
		fmt.Println("Пример: test_patterns 1c_data.db 50")
		os.Exit(1)
	}

	dbPath := os.Args[1]
	limit := 50
	if len(os.Args) >= 3 {
		fmt.Sscanf(os.Args[2], "%d", &limit)
	}

	// Открываем базу данных
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Fatalf("Ошибка открытия БД: %v", err)
	}
	defer db.Close()

	// Создаем обертку database.DB
	dbWrapper, err := database.NewDB(dbPath)
	if err != nil {
		log.Fatalf("Ошибка создания обертки БД: %v", err)
	}
	defer dbWrapper.Close()

	// Инициализируем детектор паттернов
	patternDetector := normalization.NewPatternDetector()

	// Инициализируем AI интегратор (если доступен)
	var aiIntegrator *normalization.PatternAIIntegrator
	apiKey := os.Getenv("ARLIAI_API_KEY")
	if apiKey != "" {
		aiNormalizer := normalization.NewAINormalizer(apiKey)
		aiIntegrator = normalization.NewPatternAIIntegrator(patternDetector, aiNormalizer)
		fmt.Println("✓ AI интегратор инициализирован")
	} else {
		fmt.Println("⚠ ARLIAI_API_KEY не установлен, AI отключен")
	}

	// Получаем названия из базы
	query := fmt.Sprintf(`
		SELECT DISTINCT name 
		FROM catalog_items 
		WHERE name IS NOT NULL AND name != ''
		LIMIT %d
	`, limit)

	rows, err := db.Query(query)
	if err != nil {
		log.Fatalf("Ошибка запроса: %v", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			continue
		}
		names = append(names, name)
	}

	fmt.Printf("\nНайдено %d названий для анализа\n\n", len(names))

	// Анализируем каждое название
	results := make([]map[string]interface{}, 0)

	for i, name := range names {
		fmt.Printf("[%d/%d] Анализ: %s\n", i+1, len(names), name)

		// Обнаруживаем паттерны
		matches := patternDetector.DetectPatterns(name)
		
		result := map[string]interface{}{
			"original_name":    name,
			"patterns_found":   len(matches),
			"patterns":         matches,
		}

		// Применяем алгоритмические исправления
		algorithmicFix := patternDetector.ApplyFixes(name, matches)
		result["algorithmic_fix"] = algorithmicFix

		// Если есть AI интегратор, получаем предложение с AI
		if aiIntegrator != nil {
			aiResult, err := aiIntegrator.SuggestCorrectionWithAI(name)
			if err == nil {
				result["ai_suggested_fix"] = aiResult.AISuggestedFix
				result["final_suggestion"] = aiResult.FinalSuggestion
				result["confidence"] = aiResult.Confidence
				result["reasoning"] = aiResult.Reasoning
				result["requires_review"] = aiResult.RequiresReview
			} else {
				result["ai_error"] = err.Error()
			}
		}

		// Выводим краткую информацию
		if len(matches) > 0 {
			fmt.Printf("  → Найдено паттернов: %d\n", len(matches))
			for _, match := range matches {
				fmt.Printf("    - [%s] %s: '%s'\n", match.Severity, match.Description, match.MatchedText)
			}
			fmt.Printf("  → Алгоритмическое исправление: %s\n", algorithmicFix)
			if aiIntegrator != nil {
				if aiRes, ok := result["final_suggestion"].(string); ok {
					fmt.Printf("  → Финальное предложение: %s\n", aiRes)
				}
			}
		} else {
			fmt.Printf("  ✓ Проблемных паттернов не найдено\n")
		}

		fmt.Println()

		results = append(results, result)
	}

	// Сохраняем результаты в JSON
	outputFile := "pattern_test_results.json"
	jsonData, err := json.MarshalIndent(results, "", "  ")
	if err != nil {
		log.Printf("Ошибка сериализации JSON: %v", err)
	} else {
		if err := os.WriteFile(outputFile, jsonData, 0644); err != nil {
			log.Printf("Ошибка записи файла: %v", err)
		} else {
			fmt.Printf("Результаты сохранены в %s\n", outputFile)
		}
	}

	// Выводим статистику
	fmt.Println("\n=== СТАТИСТИКА ===")
	totalPatterns := 0
	patternTypes := make(map[normalization.PatternType]int)
	severityCount := make(map[string]int)
	autoFixableCount := 0

	for _, result := range results {
		if patterns, ok := result["patterns"].([]normalization.PatternMatch); ok {
			totalPatterns += len(patterns)
			for _, match := range patterns {
				patternTypes[match.Type]++
				severityCount[match.Severity]++
				if match.AutoFixable {
					autoFixableCount++
				}
			}
		}
	}

	fmt.Printf("Всего найдено паттернов: %d\n", totalPatterns)
	fmt.Printf("Автоприменяемых: %d\n", autoFixableCount)
	fmt.Printf("\nПо типам:\n")
	for ptype, count := range patternTypes {
		fmt.Printf("  %s: %d\n", ptype, count)
	}
	fmt.Printf("\nПо серьезности:\n")
	for severity, count := range severityCount {
		fmt.Printf("  %s: %d\n", severity, count)
	}
}

