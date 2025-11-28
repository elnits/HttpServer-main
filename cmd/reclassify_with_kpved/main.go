package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"httpserver/classification"
	"httpserver/database"
	"httpserver/server"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Println("Использование: reclassify_with_kpved <путь_к_базе.db> <classifier_id> [strategy_id] [limit]")
		fmt.Println("Пример: reclassify_with_kpved 1c_data.db 1 top_priority")
		fmt.Println("Пример: reclassify_with_kpved 1c_data.db 1 top_priority 100")
		os.Exit(1)
	}

	dbPath := os.Args[1]
	classifierIDStr := os.Args[2]

	var classifierID int
	fmt.Sscanf(classifierIDStr, "%d", &classifierID)

	strategyID := "top_priority"
	if len(os.Args) >= 4 {
		strategyID = os.Args[3]
	}

	limit := 0
	if len(os.Args) >= 5 {
		fmt.Sscanf(os.Args[4], "%d", &limit)
	}

	fmt.Printf("Переклассификация с использованием КПВЭД\n")
	fmt.Printf("База данных: %s\n", dbPath)
	fmt.Printf("Classifier ID: %d\n", classifierID)
	fmt.Printf("Strategy ID: %s\n", strategyID)
	if limit > 0 {
		fmt.Printf("Лимит: %d элементов\n", limit)
	}
	fmt.Println()

	// Подключаемся к базе
	db, err := database.NewDB(dbPath)
	if err != nil {
		log.Fatalf("Ошибка подключения к базе: %v", err)
	}
	defer db.Close()

	// Получаем классификатор
	classifier, err := db.GetCategoryClassifier(classifierID)
	if err != nil {
		log.Fatalf("Ошибка получения классификатора: %v", err)
	}

	fmt.Printf("Классификатор: %s\n", classifier.Name)
	fmt.Printf("Максимальная глубина: %d\n", classifier.MaxDepth)
	fmt.Println()

	// Парсим дерево классификатора
	var classifierTree classification.CategoryNode
	if err := json.Unmarshal([]byte(classifier.TreeStructure), &classifierTree); err != nil {
		log.Fatalf("Ошибка парсинга дерева классификатора: %v", err)
	}

	// Создаем менеджер конфигурации для получения модели
	configManager := server.NewWorkerConfigManager()
	
	// Получаем API ключ и модель из конфигурации
	apiKey, model, err := configManager.GetModelAndAPIKey()
	if err != nil {
		// Fallback на переменные окружения
		apiKey = os.Getenv("ARLIAI_API_KEY")
		if apiKey == "" {
			log.Fatal("ARLIAI_API_KEY не установлен в переменных окружения")
		}
		model = os.Getenv("ARLIAI_MODEL")
		if model == "" {
			model = "GLM-4.5-Air"
		}
	}

	aiClassifier := classification.NewAIClassifier(apiKey, model)
	aiClassifier.SetClassifierTree(&classifierTree)

	// Создаем менеджер стратегий
	strategyManager := classification.NewStrategyManager()

	// Получаем нормализованные записи для переклассификации
	fmt.Println("Загрузка нормализованных записей...")
	query := `
		SELECT id, source_name, normalized_name, code, category
		FROM normalized_data
		WHERE source_name IS NOT NULL AND source_name != ''
		ORDER BY id
	`
	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := db.Query(query)
	if err != nil {
		log.Fatalf("Ошибка запроса: %v", err)
	}
	defer rows.Close()

	type Item struct {
		ID            int
		SourceName    string
		NormalizedName string
		Code          string
		OldCategory   string
	}

	var items []Item
	for rows.Next() {
		var item Item
		if err := rows.Scan(&item.ID, &item.SourceName, &item.NormalizedName, &item.Code, &item.OldCategory); err != nil {
			log.Printf("Ошибка сканирования: %v", err)
			continue
		}
		items = append(items, item)
	}

	totalItems := len(items)
	fmt.Printf("Найдено записей для переклассификации: %d\n", totalItems)
	fmt.Println()

	if totalItems == 0 {
		fmt.Println("Записи не найдены!")
		os.Exit(0)
	}

	// Переклассифицируем
	fmt.Println("Начинаем переклассификацию с использованием КПВЭД...")
	startTime := time.Now()

	successCount := 0
	errorCount := 0
	skippedCount := 0

	for i, item := range items {
		// Классифицируем с помощью AI и КПВЭД
		aiRequest := classification.AIClassificationRequest{
			ItemName:    item.SourceName,
			Description: item.Code,
			MaxLevels:   classifier.MaxDepth,
		}

		aiResponse, err := aiClassifier.ClassifyWithAI(aiRequest)
		if err != nil {
			log.Printf("Ошибка классификации для %s (ID: %d): %v", item.SourceName, item.ID, err)
			errorCount++
			if (i+1)%100 == 0 {
				fmt.Printf("Обработано: %d/%d (успешно: %d, ошибок: %d, пропущено: %d)\n",
					i+1, totalItems, successCount, errorCount, skippedCount)
			}
			continue
		}

		// Сворачиваем категорию
		foldedPath, err := strategyManager.FoldCategory(aiResponse.CategoryPath, strategyID)
		if err != nil {
			foldedPath = classification.FoldCategoryPathSimple(aiResponse.CategoryPath, 2, "top")
		}

		// Формируем новую категорию из КПВЭД (берем первый уровень)
		newCategory := ""
		if len(foldedPath) > 0 {
			newCategory = foldedPath[0]
		}
		if len(foldedPath) > 1 {
			newCategory = foldedPath[0] + " / " + foldedPath[1]
		}

		// Обновляем запись в normalized_data
		// Обновляем category на основе КПВЭД
		updateQuery := `
			UPDATE normalized_data
			SET category = ?,
			    kpved_code = ?,
			    kpved_name = ?,
			    kpved_confidence = ?
			WHERE id = ?
		`

		kpvedCode := ""
		kpvedName := ""
		if len(aiResponse.CategoryPath) > 0 {
			// Берем последний элемент пути как код КПВЭД (если есть)
			kpvedName = aiResponse.CategoryPath[len(aiResponse.CategoryPath)-1]
		}

		_, err = db.Exec(updateQuery, newCategory, kpvedCode, kpvedName, aiResponse.Confidence, item.ID)
		if err != nil {
			log.Printf("Ошибка обновления для %s (ID: %d): %v", item.SourceName, item.ID, err)
			errorCount++
			continue
		}

		successCount++

		// Прогресс каждые 10 элементов
		if (i+1)%10 == 0 {
			elapsed := time.Since(startTime)
			rate := float64(i+1) / elapsed.Seconds()
			remaining := float64(totalItems-i-1) / rate
			fmt.Printf("Обработано: %d/%d (успешно: %d, ошибок: %d) | Скорость: %.1f/сек | Осталось: ~%.0f сек\n",
				i+1, totalItems, successCount, errorCount, rate, remaining)
		}

		// Небольшая задержка
		if (i+1)%5 == 0 {
			time.Sleep(200 * time.Millisecond)
		}
	}

	elapsed := time.Since(startTime)
	fmt.Println()
	fmt.Println("=== Результаты переклассификации ===")
	fmt.Printf("Всего записей: %d\n", totalItems)
	fmt.Printf("Успешно переклассифицировано: %d\n", successCount)
	fmt.Printf("Ошибок: %d\n", errorCount)
	fmt.Printf("Пропущено: %d\n", skippedCount)
	fmt.Printf("Время выполнения: %v\n", elapsed)
	if successCount > 0 {
		fmt.Printf("Средняя скорость: %.2f элементов/сек\n", float64(successCount)/elapsed.Seconds())
	}
}

