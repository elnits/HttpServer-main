package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"httpserver/classification"
)

// ReclassificationStatus статус процесса переклассификации
type ReclassificationStatus struct {
	IsRunning   bool     `json:"isRunning"`
	Progress    float64  `json:"progress"`
	Processed   int      `json:"processed"`
	Total       int      `json:"total"`
	Success     int      `json:"success"`
	Errors      int      `json:"errors"`
	Skipped     int      `json:"skipped"`
	CurrentStep string   `json:"currentStep"`
	Logs        []string `json:"logs"`
	StartTime   string   `json:"startTime,omitempty"`
	ElapsedTime string   `json:"elapsedTime,omitempty"`
	Rate        float64  `json:"rate"` // записей в секунду
}

// ReclassificationRequest запрос на запуск переклассификации
type ReclassificationRequest struct {
	ClassifierID int    `json:"classifier_id"`
	StrategyID   string `json:"strategy_id"`
	Limit        int    `json:"limit,omitempty"` // 0 = без лимита
}

var (
	reclassificationEvents   chan string
	reclassificationRunning  bool
	reclassificationMutex    sync.RWMutex
	reclassificationStatus   ReclassificationStatus
	reclassificationStatusMutex sync.RWMutex
)

func init() {
	reclassificationEvents = make(chan string, 1000)
	reclassificationStatus = ReclassificationStatus{
		IsRunning: false,
		Logs:      make([]string, 0),
	}
}

// handleReclassificationStart запускает процесс переклассификации
func (s *Server) handleReclassificationStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	reclassificationMutex.Lock()
	if reclassificationRunning {
		reclassificationMutex.Unlock()
		s.writeJSONError(w, "Переклассификация уже выполняется", http.StatusConflict)
		return
	}
	reclassificationRunning = true
	reclassificationMutex.Unlock()

	var req ReclassificationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		reclassificationMutex.Lock()
		reclassificationRunning = false
		reclassificationMutex.Unlock()
		s.writeJSONError(w, fmt.Sprintf("Ошибка парсинга запроса: %v", err), http.StatusBadRequest)
		return
	}

	// Валидация
	if req.ClassifierID <= 0 {
		req.ClassifierID = 1 // По умолчанию КПВЭД
	}
	if req.StrategyID == "" {
		req.StrategyID = "top_priority"
	}

	// Запускаем переклассификацию в отдельной горутине
	go s.runReclassification(req)

	s.writeJSONResponse(w, map[string]interface{}{
		"success": true,
		"message": "Переклассификация запущена",
		"classifier_id": req.ClassifierID,
		"strategy_id": req.StrategyID,
		"limit": req.Limit,
	}, http.StatusOK)
}

// handleReclassificationEvents обрабатывает SSE соединение для событий переклассификации
func (s *Server) handleReclassificationEvents(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Cache-Control")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// Отправляем начальное событие
	fmt.Fprintf(w, "data: %s\n\n", `{"type":"connected","message":"Connected to reclassification events"}`)
	flusher.Flush()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case event := <-reclassificationEvents:
			eventJSON := fmt.Sprintf(`{"type":"log","message":%q,"timestamp":%q}`,
				event, time.Now().Format(time.RFC3339))
			if _, err := fmt.Fprintf(w, "data: %s\n\n", eventJSON); err != nil {
				log.Printf("Ошибка отправки SSE события: %v", err)
				return
			}
			flusher.Flush()
		case <-ticker.C:
			// Heartbeat
			if _, err := fmt.Fprintf(w, ": heartbeat\n\n"); err != nil {
				log.Printf("Ошибка отправки heartbeat: %v", err)
				return
			}
			flusher.Flush()
		case <-r.Context().Done():
			log.Printf("SSE клиент отключился: %v", r.Context().Err())
			return
		}
	}
}

// handleReclassificationStatus возвращает текущий статус переклассификации
func (s *Server) handleReclassificationStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	reclassificationStatusMutex.RLock()
	status := reclassificationStatus
	reclassificationStatusMutex.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// handleReclassificationStop останавливает процесс переклассификации
func (s *Server) handleReclassificationStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	reclassificationMutex.Lock()
	wasRunning := reclassificationRunning
	reclassificationRunning = false
	reclassificationMutex.Unlock()

	if !wasRunning {
		s.writeJSONError(w, "Переклассификация не выполняется", http.StatusBadRequest)
		return
	}

	s.sendReclassificationEvent("⚠ Процесс переклассификации остановлен пользователем")

	s.writeJSONResponse(w, map[string]interface{}{
		"success": true,
		"message": "Переклассификация остановлена",
	}, http.StatusOK)
}

// runReclassification выполняет переклассификацию
func (s *Server) runReclassification(req ReclassificationRequest) {
	defer func() {
		reclassificationMutex.Lock()
		reclassificationRunning = false
		reclassificationMutex.Unlock()

		reclassificationStatusMutex.Lock()
		reclassificationStatus.IsRunning = false
		reclassificationStatusMutex.Unlock()

		s.sendReclassificationEvent("✅ Переклассификация завершена")
	}()

	startTime := time.Now()

	// Инициализация статуса
	reclassificationStatusMutex.Lock()
	reclassificationStatus = ReclassificationStatus{
		IsRunning:   true,
		Processed:   0,
		Total:       0,
		Success:     0,
		Errors:      0,
		Skipped:     0,
		CurrentStep: "Инициализация...",
		Logs:        make([]string, 0),
		StartTime:   startTime.Format(time.RFC3339),
	}
	reclassificationStatusMutex.Unlock()

	s.sendReclassificationEvent("🚀 Запуск переклассификации с использованием КПВЭД")
	s.sendReclassificationEvent(fmt.Sprintf("📋 Классификатор ID: %d", req.ClassifierID))
	s.sendReclassificationEvent(fmt.Sprintf("📊 Стратегия: %s", req.StrategyID))
	if req.Limit > 0 {
		s.sendReclassificationEvent(fmt.Sprintf("🔢 Лимит: %d записей", req.Limit))
	}

	// Получаем классификатор (из основной БД, где хранятся классификаторы)
	classifier, err := s.db.GetCategoryClassifier(req.ClassifierID)
	if err != nil {
		s.sendReclassificationEvent(fmt.Sprintf("❌ Ошибка получения классификатора: %v", err))
		return
	}

	s.sendReclassificationEvent(fmt.Sprintf("✅ Классификатор загружен: %s (глубина: %d)", classifier.Name, classifier.MaxDepth))

	// Парсим дерево классификатора
	var classifierTree classification.CategoryNode
	if err := json.Unmarshal([]byte(classifier.TreeStructure), &classifierTree); err != nil {
		s.sendReclassificationEvent(fmt.Sprintf("❌ Ошибка парсинга дерева классификатора: %v", err))
		return
	}

	// Получаем API ключ и модель из WorkerConfigManager
	var apiKey string
	if s.workerConfigManager != nil {
		provider, err := s.workerConfigManager.GetActiveProvider()
		if err == nil {
			apiKey = provider.APIKey
		}
	}
	
	// Fallback на переменные окружения, если WorkerConfigManager не доступен
	if apiKey == "" {
		apiKey = os.Getenv("ARLIAI_API_KEY")
		if apiKey == "" {
			s.sendReclassificationEvent("❌ ARLIAI_API_KEY не установлен в переменных окружения")
			s.sendReclassificationEvent("💡 Установите переменную окружения ARLIAI_API_KEY для работы AI классификации")
			return
		}
	}
	
	if len(apiKey) < 10 {
		s.sendReclassificationEvent(fmt.Sprintf("⚠️  ARLIAI_API_KEY кажется слишком коротким (%d символов)", len(apiKey)))
	}

	// Получаем модель из WorkerConfigManager
	model := s.getModelFromConfig()

	aiClassifier := classification.NewAIClassifier(apiKey, model)
	aiClassifier.SetClassifierTree(&classifierTree)

	// Создаем менеджер стратегий
	strategyManager := classification.NewStrategyManager()

	// Получаем нормализованные записи из основной БД (1c_data.db)
	// где реально хранятся данные normalized_data
	s.sendReclassificationEvent("📥 Загрузка нормализованных записей...")

	query := `
		SELECT id, source_name, normalized_name, code, category
		FROM normalized_data
		WHERE source_name IS NOT NULL AND source_name != ''
		ORDER BY id
	`
	if req.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", req.Limit)
	}

	rows, err := s.db.Query(query)
	if err != nil {
		s.sendReclassificationEvent(fmt.Sprintf("❌ Ошибка запроса: %v", err))
		return
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
	s.sendReclassificationEvent(fmt.Sprintf("✅ Найдено записей для переклассификации: %d", totalItems))

	if totalItems == 0 {
		s.sendReclassificationEvent("⚠ Записи не найдены!")
		return
	}

	// Обновляем статус
	reclassificationStatusMutex.Lock()
	reclassificationStatus.Total = totalItems
	reclassificationStatus.CurrentStep = "Выполняется переклассификация..."
	reclassificationStatusMutex.Unlock()

	// Переклассифицируем
	s.sendReclassificationEvent("🔄 Начинаем переклассификацию...")

	successCount := 0
	errorCount := 0
	skippedCount := 0

	for i, item := range items {
		// Проверяем, не остановлен ли процесс
		reclassificationMutex.RLock()
		shouldStop := !reclassificationRunning
		reclassificationMutex.RUnlock()

		if shouldStop {
			s.sendReclassificationEvent("⚠ Процесс остановлен пользователем")
			break
		}

		// Классифицируем с помощью AI и КПВЭД
		aiRequest := classification.AIClassificationRequest{
			ItemName:    item.SourceName,
			Description: item.Code,
			MaxLevels:   classifier.MaxDepth,
		}

		aiResponse, err := aiClassifier.ClassifyWithAI(aiRequest)
		if err != nil {
			// Детальная информация об ошибке
			errorDetails := err.Error()
			errorMsg := fmt.Sprintf("❌ Ошибка классификации для '%s' (ID: %d): %s", 
				item.SourceName, item.ID, errorDetails)
			log.Printf("RECLASSIFICATION ERROR: %s", errorMsg)
			s.sendReclassificationEvent(errorMsg)
			
			// Если это первые несколько ошибок, добавляем подсказки
			if errorCount < 3 {
				if strings.Contains(errorDetails, "API") || strings.Contains(errorDetails, "connection") || strings.Contains(errorDetails, "timeout") {
					s.sendReclassificationEvent("💡 Проверьте подключение к AI сервису и ARLIAI_API_KEY")
				} else if strings.Contains(errorDetails, "parse") || strings.Contains(errorDetails, "JSON") {
					s.sendReclassificationEvent("💡 Проблема с форматом ответа от AI. Проверьте настройки модели.")
				}
			}
			
			errorCount++

			reclassificationStatusMutex.Lock()
			reclassificationStatus.Processed++
			reclassificationStatus.Errors = errorCount
			reclassificationStatus.Progress = float64(reclassificationStatus.Processed) / float64(totalItems) * 100
			reclassificationStatusMutex.Unlock()

			if (i+1)%10 == 0 {
				elapsed := time.Since(startTime)
				rate := float64(i+1) / elapsed.Seconds()
				s.sendReclassificationEvent(fmt.Sprintf("📊 Обработано: %d/%d (успешно: %d, ошибок: %d) | Скорость: %.1f/сек",
					i+1, totalItems, successCount, errorCount, rate))
			}
			continue
		}

		// Сворачиваем категорию
		foldedPath, err := strategyManager.FoldCategory(aiResponse.CategoryPath, req.StrategyID)
		if err != nil {
			foldedPath = classification.FoldCategoryPathSimple(aiResponse.CategoryPath, 2, "top")
		}

		// Формируем новую категорию из КПВЭД
		newCategory := ""
		if len(foldedPath) > 0 {
			newCategory = foldedPath[0]
		}
		if len(foldedPath) > 1 {
			newCategory = foldedPath[0] + " / " + foldedPath[1]
		}

		// Обновляем запись в normalized_data
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
			kpvedName = aiResponse.CategoryPath[len(aiResponse.CategoryPath)-1]
		}

		// Обновляем запись в основной БД (1c_data.db), где реально хранятся данные
		_, err = s.db.Exec(updateQuery, newCategory, kpvedCode, kpvedName, aiResponse.Confidence, item.ID)
		if err != nil {
			errorMsg := fmt.Sprintf("❌ Ошибка обновления для '%s' (ID: %d): %v", item.SourceName, item.ID, err)
			log.Printf("%s", errorMsg)
			s.sendReclassificationEvent(errorMsg)
			errorCount++

			reclassificationStatusMutex.Lock()
			reclassificationStatus.Processed++
			reclassificationStatus.Errors = errorCount
			reclassificationStatus.Progress = float64(reclassificationStatus.Processed) / float64(totalItems) * 100
			reclassificationStatusMutex.Unlock()

			continue
		}

		successCount++

		// Обновляем статус
		elapsed := time.Since(startTime)
		reclassificationStatusMutex.Lock()
		reclassificationStatus.Processed = i + 1
		reclassificationStatus.Success = successCount
		reclassificationStatus.Errors = errorCount
		reclassificationStatus.Skipped = skippedCount
		reclassificationStatus.Progress = float64(i+1) / float64(totalItems) * 100
		reclassificationStatus.ElapsedTime = elapsed.String()
		if elapsed.Seconds() > 0 {
			reclassificationStatus.Rate = float64(i+1) / elapsed.Seconds()
		}
		reclassificationStatusMutex.Unlock()

		// Прогресс каждые 10 элементов
		if (i+1)%10 == 0 {
			remaining := float64(totalItems-i-1) / reclassificationStatus.Rate
			s.sendReclassificationEvent(fmt.Sprintf("📊 Обработано: %d/%d (успешно: %d, ошибок: %d) | Скорость: %.1f/сек | Осталось: ~%.0f сек",
				i+1, totalItems, successCount, errorCount, reclassificationStatus.Rate, remaining))
		}

		// Небольшая задержка
		if (i+1)%5 == 0 {
			time.Sleep(200 * time.Millisecond)
		}
	}

	elapsed := time.Since(startTime)
	s.sendReclassificationEvent(fmt.Sprintf("✅ Переклассификация завершена за %v", elapsed))
	s.sendReclassificationEvent(fmt.Sprintf("📊 Всего записей: %d", totalItems))
	s.sendReclassificationEvent(fmt.Sprintf("✅ Успешно переклассифицировано: %d", successCount))
	s.sendReclassificationEvent(fmt.Sprintf("❌ Ошибок: %d", errorCount))
	s.sendReclassificationEvent(fmt.Sprintf("⏭️  Пропущено: %d", skippedCount))
	if successCount > 0 {
		s.sendReclassificationEvent(fmt.Sprintf("⚡ Средняя скорость: %.2f элементов/сек", float64(successCount)/elapsed.Seconds()))
	}
}

// sendReclassificationEvent отправляет событие в канал
func (s *Server) sendReclassificationEvent(message string) {
	// Всегда логируем ошибки, даже если канал переполнен
	isError := strings.Contains(message, "❌") || strings.Contains(message, "Ошибка") || strings.Contains(message, "ошибка")
	if isError {
		log.Printf("RECLASSIFICATION ERROR: %s", message)
	}

	select {
	case reclassificationEvents <- message:
		// Событие отправлено
		reclassificationStatusMutex.Lock()
		reclassificationStatus.Logs = append(reclassificationStatus.Logs, message)
		// Ограничиваем размер логов
		if len(reclassificationStatus.Logs) > 1000 {
			reclassificationStatus.Logs = reclassificationStatus.Logs[len(reclassificationStatus.Logs)-1000:]
		}
		reclassificationStatus.CurrentStep = message
		reclassificationStatusMutex.Unlock()
	default:
		// Канал переполнен, но для ошибок все равно логируем
		if isError {
			log.Printf("Канал переполнен, но ошибка важна: %s", message)
		} else {
			log.Printf("Канал событий переклассификации переполнен, пропускаем: %s", message)
		}
	}
}

