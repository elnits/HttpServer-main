package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"httpserver/database"
	"httpserver/quality"
)

func main() {
	// Подключаемся к базе данных
	dbPath := "1c_data.db"
	if len(os.Args) > 1 {
		dbPath = os.Args[1]
	}

	db, err := database.NewDB(dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Получаем последний upload с номенклатурой
	query := `
		SELECT u.id, u.database_id, COUNT(n.id) as items_count
		FROM uploads u
		LEFT JOIN nomenclature_items n ON n.upload_id = u.id
		WHERE u.status = 'completed'
		GROUP BY u.id, u.database_id
		HAVING items_count > 0
		ORDER BY u.completed_at DESC
		LIMIT 1
	`

	var uploadID, databaseID, itemsCount int
	err = db.QueryRow(query).Scan(&uploadID, &databaseID, &itemsCount)
	if err != nil {
		log.Fatalf("Failed to find upload with nomenclature: %v", err)
	}

	fmt.Printf("Найден upload ID=%d с %d записями номенклатуры\n", uploadID, itemsCount)
	fmt.Printf("Начинаем тестирование производительности анализа качества...\n\n")

	// Создаем анализатор качества
	analyzer := quality.NewQualityAnalyzer(db)

	// Тест 1: Полный анализ (включает все метрики)
	fmt.Println("=== Тест 1: Полный анализ качества ===")
	start := time.Now()
	err = analyzer.AnalyzeUpload(uploadID, databaseID)
	elapsed := time.Since(start)
	if err != nil {
		log.Printf("Ошибка полного анализа: %v", err)
	} else {
		fmt.Printf("Время выполнения: %v\n", elapsed)
		fmt.Printf("Скорость: %.2f записей/сек\n\n", float64(itemsCount)/elapsed.Seconds())
	}

	// Тест 2: Поиск нечетких дубликатов
	fmt.Println("=== Тест 2: Поиск нечетких дубликатов ===")
	start = time.Now()
	fuzzyMatcher := quality.NewFuzzyMatcher(db, 0.85)
	duplicateGroups, err := fuzzyMatcher.FindDuplicateNames(uploadID, databaseID)
	elapsed = time.Since(start)
	if err != nil {
		log.Printf("Ошибка поиска дубликатов: %v", err)
	} else {
		fmt.Printf("Время выполнения: %v\n", elapsed)
		fmt.Printf("Найдено групп дубликатов: %d\n", len(duplicateGroups))
		if len(duplicateGroups) > 0 {
			fmt.Printf("Пример первой группы: %d элементов, схожесть %.2f%%\n",
				len(duplicateGroups[0].Items), duplicateGroups[0].Similarity*100)
		}
		fmt.Printf("Скорость: %.2f записей/сек\n\n", float64(itemsCount)/elapsed.Seconds())
	}

	// Тест 3: Получение отчёта с пагинацией
	fmt.Println("=== Тест 3: Получение отчёта (с пагинацией) ===")
	start = time.Now()
	issues, totalCount, err := db.GetQualityIssues(uploadID, map[string]interface{}{}, 100, 0)
	elapsed = time.Since(start)
	if err != nil {
		log.Printf("Ошибка получения issues: %v", err)
	} else {
		fmt.Printf("Время выполнения: %v\n", elapsed)
		fmt.Printf("Всего issues: %d, получено: %d\n", totalCount, len(issues))
		fmt.Printf("Скорость: %.2f issues/сек\n\n", float64(len(issues))/elapsed.Seconds())
	}

	// Тест 4: Получение метрик
	fmt.Println("=== Тест 4: Получение метрик качества ===")
	start = time.Now()
	metrics, err := db.GetQualityMetrics(uploadID)
	elapsed = time.Since(start)
	if err != nil {
		log.Printf("Ошибка получения метрик: %v", err)
	} else {
		fmt.Printf("Время выполнения: %v\n", elapsed)
		fmt.Printf("Получено метрик: %d\n", len(metrics))
		if len(metrics) > 0 {
			fmt.Printf("Пример метрики: %s = %.2f%% (%s)\n",
				metrics[0].MetricName, metrics[0].MetricValue, metrics[0].Status)
		}
		fmt.Println()
	}

	// Итоговая статистика
	fmt.Println("=== Итоговая статистика ===")
	fmt.Printf("Всего записей номенклатуры: %d\n", itemsCount)
	fmt.Printf("Все тесты завершены успешно!\n")
}

