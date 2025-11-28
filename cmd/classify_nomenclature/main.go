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
		fmt.Println("Использование: classify_nomenclature <путь_к_базе.db> <classifier_id> [strategy_id] [client_id] [project_id]")
		fmt.Println("Пример: classify_nomenclature 1c_data.db 1 top_priority")
		fmt.Println("Пример: classify_nomenclature 1c_data.db 1 top_priority 1 1")
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

	var clientID *int
	var projectID *int

	if len(os.Args) >= 5 {
		id := 0
		fmt.Sscanf(os.Args[4], "%d", &id)
		if id > 0 {
			clientID = &id
		}
	}

	if len(os.Args) >= 6 {
		id := 0
		fmt.Sscanf(os.Args[5], "%d", &id)
		if id > 0 {
			projectID = &id
		}
	}

	fmt.Printf("Классификация номенклатуры\n")
	fmt.Printf("База данных: %s\n", dbPath)
	fmt.Printf("Classifier ID: %d\n", classifierID)
	fmt.Printf("Strategy ID: %s\n", strategyID)
	if clientID != nil {
		fmt.Printf("Client ID: %d\n", *clientID)
	}
	if projectID != nil {
		fmt.Printf("Project ID: %d\n", *projectID)
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

	// Создаем AI классификатор
	apiKey := os.Getenv("ARLIAI_API_KEY")
	if apiKey == "" {
		log.Fatal("ARLIAI_API_KEY не установлен в переменных окружения")
	}

	// Создаем менеджер конфигурации для получения модели
	configManager := server.NewWorkerConfigManager()
	
	// Получаем модель из конфигурации
	_, model, err := configManager.GetModelAndAPIKey()
	if err != nil {
		// Fallback на переменные окружения
		model = os.Getenv("ARLIAI_MODEL")
		if model == "" {
			model = "GLM-4.5-Air"
		}
	}

	aiClassifier := classification.NewAIClassifier(apiKey, model)
	aiClassifier.SetClassifierTree(&classifierTree)

	// Создаем менеджер стратегий
	strategyManager := classification.NewStrategyManager()

	// Получаем все номенклатуры
	fmt.Println("Загрузка номенклатуры из базы...")
	
	// Получаем общее количество
	totalItems, err := db.GetNomenclatureItemsCount()
	if err != nil {
		log.Fatalf("Ошибка получения количества номенклатур: %v", err)
	}

	// Загружаем порциями
	batchSize := 1000
	var allItems []struct {
		ID       int
		Ref      string
		Code     string
		Name     string
	}

	for offset := 0; offset < totalItems; offset += batchSize {
		batch, err := db.GetNomenclatureItemsForClassification(batchSize, offset)
		if err != nil {
			log.Fatalf("Ошибка загрузки номенклатуры: %v", err)
		}
		allItems = append(allItems, batch...)
	}

	items := allItems
	totalItems = len(items)
	fmt.Printf("Найдено номенклатур: %d\n", totalItems)
	fmt.Println()

	if totalItems == 0 {
		fmt.Println("Номенклатура не найдена!")
		os.Exit(0)
	}

	// Классифицируем
	fmt.Println("Начинаем классификацию...")
	startTime := time.Now()

	successCount := 0
	errorCount := 0
	skippedCount := 0

	for i, item := range items {
		// Пропускаем уже классифицированные
		hasClassification, err := db.HasNomenclatureClassification(item.ID)
		if err == nil && hasClassification {
			skippedCount++
			if (i+1)%100 == 0 {
				fmt.Printf("Обработано: %d/%d (успешно: %d, ошибок: %d, пропущено: %d)\n",
					i+1, totalItems, successCount, errorCount, skippedCount)
			}
			continue
		}

		// Классифицируем
		aiRequest := classification.AIClassificationRequest{
			ItemName:    item.Name,
			Description: item.Code,
			MaxLevels:   classifier.MaxDepth,
		}

		aiResponse, err := aiClassifier.ClassifyWithAI(aiRequest)
		if err != nil {
			log.Printf("Ошибка классификации для %s (ID: %d): %v", item.Name, item.ID, err)
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
			// Используем простую свертку
			foldedPath = classification.FoldCategoryPathSimple(aiResponse.CategoryPath, 2, "top")
		}

		// Сохраняем классификацию
		categoryLevels := make(map[string]string)
		for i, level := range foldedPath {
			categoryLevels[fmt.Sprintf("level%d", i+1)] = level
		}

		if err := db.UpdateNomenclatureItemClassification(item.ID, aiResponse.CategoryPath, categoryLevels, strategyID, aiResponse.Confidence); err != nil {
			log.Printf("Ошибка сохранения классификации для %s (ID: %d): %v", item.Name, item.ID, err)
			errorCount++
			continue
		}

		successCount++

		// Прогресс каждые 100 элементов
		if (i+1)%100 == 0 {
			elapsed := time.Since(startTime)
			rate := float64(i+1) / elapsed.Seconds()
			remaining := float64(totalItems-i-1) / rate
			fmt.Printf("Обработано: %d/%d (успешно: %d, ошибок: %d, пропущено: %d) | Скорость: %.1f/сек | Осталось: ~%.0f сек\n",
				i+1, totalItems, successCount, errorCount, skippedCount, rate, remaining)
		}

		// Небольшая задержка для избежания rate limiting
		if (i+1)%10 == 0 {
			time.Sleep(100 * time.Millisecond)
		}
	}

	elapsed := time.Since(startTime)
	fmt.Println()
	fmt.Println("=== Результаты классификации ===")
	fmt.Printf("Всего номенклатур: %d\n", totalItems)
	fmt.Printf("Успешно классифицировано: %d\n", successCount)
	fmt.Printf("Ошибок: %d\n", errorCount)
	fmt.Printf("Пропущено (уже классифицировано): %d\n", skippedCount)
	fmt.Printf("Время выполнения: %v\n", elapsed)
	fmt.Printf("Средняя скорость: %.2f элементов/сек\n", float64(successCount)/elapsed.Seconds())
}

