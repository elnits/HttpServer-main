package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

const (
	baseURL = "http://localhost:9999"
)

type NormalizeRequest struct {
	DatabasePath      string   `json:"database_path"`
	SourceTable       string   `json:"source_table"`
	ReferenceColumn   string   `json:"reference_column"`
	CodeColumn        string   `json:"code_column"`
	NameColumn        string   `json:"name_column"`
	ProcessingLevel   string   `json:"processing_level"`
	BatchSize         int      `json:"batch_size"`
	WorkerCount       int      `json:"worker_count"`
	UseCache          bool     `json:"use_cache"`
	UseBatchProcessor bool     `json:"use_batch_processor"`
	UseCheckpoint     bool     `json:"use_checkpoint"`
}

type NormalizeResponse struct {
	Success      bool   `json:"success"`
	Message      string `json:"message"`
	ProcessingID string `json:"processing_id"`
}

type NormalizationStatus struct {
	Status          string  `json:"status"`
	Progress        float64 `json:"progress"`
	ProcessedCount  int     `json:"processed_count"`
	TotalCount      int     `json:"total_count"`
	CurrentPhase    string  `json:"current_phase"`
	ElapsedTime     float64 `json:"elapsed_time"`
	EstimatedRemain float64 `json:"estimated_remain"`
}

type StatsResponse struct {
	TotalRecords    int `json:"total_records"`
	BasicLevel      int `json:"basic_level"`
	AIEnhancedLevel int `json:"ai_enhanced_level"`
	BenchmarkLevel  int `json:"benchmark_level"`
}

func main() {
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)

	fmt.Println("=" + strings.Repeat("=", 79))
	fmt.Println("ТЕСТИРОВАНИЕ ПРОЦЕССА НОРМАЛИЗАЦИИ И ДЕДУПЛИКАЦИИ")
	fmt.Println("=" + strings.Repeat("=", 79))

	// Шаг 1: Проверка доступности сервера
	fmt.Println("\n[1/4] Проверка доступности сервера...")
	if !checkServerHealth() {
		log.Fatal("Сервер недоступен. Запустите сервер перед выполнением теста.")
	}
	fmt.Println("✓ Сервер работает")

	// Шаг 2: Запуск первой нормализации
	fmt.Println("\n[2/4] Запуск первой нормализации...")
	processingID1, err := startNormalization("BASIC")
	if err != nil {
		log.Fatalf("Ошибка запуска нормализации: %v", err)
	}
	fmt.Printf("✓ Нормализация запущена (ID: %s)\n", processingID1)

	// Ждем завершения первой нормализации
	fmt.Println("  Ожидание завершения первой нормализации...")
	if !waitForNormalization(processingID1) {
		log.Fatal("Ошибка выполнения нормализации")
	}
	fmt.Println("✓ Первая нормализация завершена")

	// Проверяем статистику после первой нормализации
	stats1 := getStats()
	fmt.Printf("\nСтатистика после первой нормализации:\n")
	fmt.Printf("  - Всего записей: %d\n", stats1.TotalRecords)
	fmt.Printf("  - Базовый уровень: %d\n", stats1.BasicLevel)

	// Шаг 3: Запуск второй нормализации (должна найти дубликаты)
	fmt.Println("\n[3/4] Запуск второй нормализации (тест дедупликации)...")
	processingID2, err := startNormalization("BASIC")
	if err != nil {
		log.Fatalf("Ошибка запуска второй нормализации: %v", err)
	}
	fmt.Printf("✓ Вторая нормализация запущена (ID: %s)\n", processingID2)

	// Ждем завершения второй нормализации
	fmt.Println("  Ожидание завершения второй нормализации...")
	if !waitForNormalization(processingID2) {
		log.Fatal("Ошибка выполнения второй нормализации")
	}
	fmt.Println("✓ Вторая нормализация завершена")

	// Проверяем статистику после второй нормализации
	stats2 := getStats()
	fmt.Printf("\nСтатистика после второй нормализации:\n")
	fmt.Printf("  - Всего записей: %d\n", stats2.TotalRecords)
	fmt.Printf("  - Базовый уровень: %d\n", stats2.BasicLevel)

	// Шаг 4: Проверка дедупликации
	fmt.Println("\n[4/4] Анализ результатов...")
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("РЕЗУЛЬТАТЫ ТЕСТА ДЕДУПЛИКАЦИИ")
	fmt.Println(strings.Repeat("=", 80))

	if stats2.TotalRecords == stats1.TotalRecords {
		fmt.Println("✓ УСПЕХ: Дедупликация работает корректно!")
		fmt.Println("  Дубликаты не были добавлены повторно в БД")
		fmt.Printf("  Количество записей осталось: %d\n", stats2.TotalRecords)
	} else {
		fmt.Println("✗ ОШИБКА: Дедупликация не сработала!")
		fmt.Printf("  Первая нормализация: %d записей\n", stats1.TotalRecords)
		fmt.Printf("  Вторая нормализация: %d записей\n", stats2.TotalRecords)
		fmt.Printf("  Разница: %d записей (дубликаты)\n", stats2.TotalRecords-stats1.TotalRecords)
	}

	// Проверяем merged_count
	fmt.Println("\nПроверка merged_count (счетчик объединенных дубликатов)...")
	checkMergedCount()

	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Println("ТЕСТ ЗАВЕРШЕН")
	fmt.Println(strings.Repeat("=", 80))
}

func checkServerHealth() bool {
	resp, err := http.Get(baseURL + "/api/monitoring/metrics")
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func startNormalization(level string) (string, error) {
	req := NormalizeRequest{
		DatabasePath:      "1c_data.db",
		SourceTable:       "catalog_items",
		ReferenceColumn:   "reference",
		CodeColumn:        "code",
		NameColumn:        "name",
		ProcessingLevel:   level,
		BatchSize:         100,
		WorkerCount:       2,
		UseCache:          true,
		UseBatchProcessor: true,
		UseCheckpoint:     true,
	}

	jsonData, _ := json.Marshal(req)
	resp, err := http.Post(
		baseURL+"/api/normalize/start",
		"application/json",
		bytes.NewBuffer(jsonData),
	)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var normResp NormalizeResponse
	if err := json.Unmarshal(body, &normResp); err != nil {
		return "", err
	}

	if !normResp.Success {
		return "", fmt.Errorf("normalization failed: %s", normResp.Message)
	}

	// Возвращаем пустую строку, т.к. API не возвращает processing_id
	return "batch", nil
}

func waitForNormalization(processingID string) bool {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	timeout := time.After(5 * time.Minute)
	lastProgress := 0.0

	for {
		select {
		case <-timeout:
			fmt.Println("\n✗ Таймаут ожидания нормализации")
			return false

		case <-ticker.C:
			resp, err := http.Get(baseURL + "/api/normalization/status")
			if err != nil {
				continue
			}

			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()

			var status NormalizationStatus
			if err := json.Unmarshal(body, &status); err != nil {
				continue
			}

			// Показываем прогресс только при изменении
			if status.Progress != lastProgress {
				fmt.Printf("\r  Прогресс: %.1f%% (%d/%d) - %s",
					status.Progress, status.ProcessedCount, status.TotalCount, status.CurrentPhase)
				lastProgress = status.Progress
			}

			if status.Status == "completed" {
				fmt.Println("\n  ✓ Нормализация завершена успешно")
				return true
			}

			if status.Status == "error" || status.Status == "failed" {
				fmt.Println("\n  ✗ Нормализация завершилась с ошибкой")
				return false
			}
		}
	}
}

func getStats() StatsResponse {
	resp, err := http.Get(baseURL + "/api/normalization/stats")
	if err != nil {
		log.Printf("Ошибка получения статистики: %v", err)
		return StatsResponse{}
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var stats StatsResponse
	json.Unmarshal(body, &stats)

	return stats
}

func checkMergedCount() {
	// Получаем записи с merged_count > 0
	resp, err := http.Get(baseURL + "/api/normalization/groups?limit=100")
	if err != nil {
		log.Printf("Ошибка получения групп: %v", err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		log.Printf("Ошибка парсинга групп: %v", err)
		return
	}

	groups, ok := result["groups"].([]interface{})
	if !ok || len(groups) == 0 {
		fmt.Println("  Нет данных о группах")
		return
	}

	mergedCount := 0
	for _, g := range groups {
		group := g.(map[string]interface{})
		if mc, ok := group["merged_count"].(float64); ok && mc > 0 {
			mergedCount++
			fmt.Printf("  Группа '%s': merged_count = %.0f\n",
				group["normalized_name"], mc)
		}
	}

	if mergedCount > 0 {
		fmt.Printf("\n✓ Найдено %d групп с объединенными дубликатами\n", mergedCount)
	} else {
		fmt.Println("\n  Нет групп с merged_count > 0")
	}
}
