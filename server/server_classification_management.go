package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
)

// ResetClassificationRequest запрос на сброс классификации
type ResetClassificationRequest struct {
	NormalizedName string  `json:"normalized_name,omitempty"`
	Category       string  `json:"category,omitempty"`
	KpvedCode      string  `json:"kpved_code,omitempty"`
	MinConfidence  float64 `json:"min_confidence,omitempty"`
	ResetAll       bool    `json:"reset_all,omitempty"`
}

// MarkIncorrectRequest запрос на пометку классификации как неправильной
type MarkIncorrectRequest struct {
	NormalizedName string `json:"normalized_name"`
	Category       string `json:"category"`
	Reason         string `json:"reason,omitempty"`
}

// KpvedStats статистика классификации
type KpvedStats struct {
	TotalRecords    int                        `json:"total_records"`
	Classified      int                        `json:"classified"`
	NotClassified   int                        `json:"not_classified"`
	LowConfidence   int                        `json:"low_confidence"`   // confidence < 0.7
	MarkedIncorrect int                        `json:"marked_incorrect"`
	ByConfidence    map[string]int              `json:"by_confidence"`
	ByCategory      map[string]CategoryStats    `json:"by_category,omitempty"`
	IncorrectItems  []IncorrectClassificationItem `json:"incorrect_items,omitempty"`
}

// CategoryStats статистика по категории
type CategoryStats struct {
	Total       int `json:"total"`
	Classified  int `json:"classified"`
	NotClassified int `json:"not_classified"`
	LowConfidence int `json:"low_confidence"`
	MarkedIncorrect int `json:"marked_incorrect"`
}

// IncorrectClassificationItem элемент с неправильной классификацией
type IncorrectClassificationItem struct {
	NormalizedName string  `json:"normalized_name"`
	Category       string  `json:"category"`
	KpvedCode      string  `json:"kpved_code"`
	KpvedName      string  `json:"kpved_name"`
	Confidence     float64 `json:"confidence"`
	Reason         string  `json:"reason,omitempty"`
}

// WorkersStatus статус воркеров
type WorkersStatus struct {
	IsRunning     bool                          `json:"is_running"`
	WorkersCount  int                           `json:"workers_count"`
	Stopped       bool                          `json:"stopped"`
	CurrentTasks  []map[string]interface{}       `json:"current_tasks"`
	TotalProcessed int                          `json:"total_processed,omitempty"`
	TotalFailed    int                          `json:"total_failed,omitempty"`
}

// handleResetClassification сбрасывает классификацию для конкретных записей
func (s *Server) handleResetClassification(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ResetClassificationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeJSONError(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	// Строим SQL запрос для сброса
	var conditions []string
	var args []interface{}

	if req.NormalizedName != "" {
		conditions = append(conditions, "normalized_name = ?")
		args = append(args, req.NormalizedName)
	}

	if req.Category != "" {
		conditions = append(conditions, "category = ?")
		args = append(args, req.Category)
	}

	if req.KpvedCode != "" {
		conditions = append(conditions, "kpved_code = ?")
		args = append(args, req.KpvedCode)
	}

	if req.MinConfidence > 0 {
		conditions = append(conditions, "kpved_confidence < ?")
		args = append(args, req.MinConfidence)
	}

	// Если reset_all, сбрасываем все
	var query string
	if req.ResetAll {
		query = `UPDATE normalized_data 
			SET kpved_code = NULL, kpved_name = NULL, kpved_confidence = 0.0,
			    validation_status = ''
			WHERE kpved_code IS NOT NULL AND kpved_code != ''`
	} else if len(conditions) > 0 {
		query = fmt.Sprintf(`UPDATE normalized_data 
			SET kpved_code = NULL, kpved_name = NULL, kpved_confidence = 0.0,
			    validation_status = ''
			WHERE kpved_code IS NOT NULL AND kpved_code != '' AND %s`,
			strings.Join(conditions, " AND "))
	} else {
		s.writeJSONError(w, "Не указаны критерии для сброса. Укажите normalized_name, category, kpved_code или установите reset_all=true", http.StatusBadRequest)
		return
	}

	result, err := s.db.Exec(query, args...)
	if err != nil {
		log.Printf("[ResetClassification] Error executing query: %v", err)
		s.writeJSONError(w, fmt.Sprintf("Failed to reset classification: %v", err), http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	log.Printf("[ResetClassification] Reset %d records", rowsAffected)

	s.writeJSONResponse(w, map[string]interface{}{
		"success":       true,
		"message":       "Классификация сброшена",
		"rows_affected": rowsAffected,
	}, http.StatusOK)
}

// handleResetAllClassification сбрасывает всю классификацию
func (s *Server) handleResetAllClassification(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	query := `UPDATE normalized_data 
		SET kpved_code = NULL, kpved_name = NULL, kpved_confidence = 0.0,
		    validation_status = ''
		WHERE kpved_code IS NOT NULL AND kpved_code != ''`

	result, err := s.db.Exec(query)
	if err != nil {
		log.Printf("[ResetAllClassification] Error: %v", err)
		s.writeJSONError(w, fmt.Sprintf("Failed to reset all classifications: %v", err), http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	log.Printf("[ResetAllClassification] Reset all %d records", rowsAffected)

	s.writeJSONResponse(w, map[string]interface{}{
		"success":       true,
		"message":       "Вся классификация сброшена",
		"rows_affected": rowsAffected,
	}, http.StatusOK)
}

// handleResetByCode сбрасывает классификацию по коду КПВЭД
func (s *Server) handleResetByCode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		KpvedCode string `json:"kpved_code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeJSONError(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	if req.KpvedCode == "" {
		s.writeJSONError(w, "kpved_code is required", http.StatusBadRequest)
		return
	}

	query := `UPDATE normalized_data 
		SET kpved_code = NULL, kpved_name = NULL, kpved_confidence = 0.0,
		    validation_status = ''
		WHERE kpved_code = ?`

	result, err := s.db.Exec(query, req.KpvedCode)
	if err != nil {
		log.Printf("[ResetByCode] Error: %v", err)
		s.writeJSONError(w, fmt.Sprintf("Failed to reset classification: %v", err), http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	log.Printf("[ResetByCode] Reset %d records with code %s", rowsAffected, req.KpvedCode)

	s.writeJSONResponse(w, map[string]interface{}{
		"success":       true,
		"message":       fmt.Sprintf("Классификация с кодом %s сброшена", req.KpvedCode),
		"rows_affected": rowsAffected,
		"kpved_code":    req.KpvedCode,
	}, http.StatusOK)
}

// handleResetLowConfidence сбрасывает классификацию с низкой уверенностью
func (s *Server) handleResetLowConfidence(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		MaxConfidence float64 `json:"max_confidence,omitempty"` // По умолчанию 0.7
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeJSONError(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	maxConfidence := req.MaxConfidence
	if maxConfidence == 0 {
		maxConfidence = 0.7 // По умолчанию
	}

	query := `UPDATE normalized_data 
		SET kpved_code = NULL, kpved_name = NULL, kpved_confidence = 0.0,
		    validation_status = ''
		WHERE kpved_code IS NOT NULL AND kpved_code != '' 
		  AND kpved_confidence < ?`

	result, err := s.db.Exec(query, maxConfidence)
	if err != nil {
		log.Printf("[ResetLowConfidence] Error: %v", err)
		s.writeJSONError(w, fmt.Sprintf("Failed to reset low confidence classifications: %v", err), http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	log.Printf("[ResetLowConfidence] Reset %d records with confidence < %.2f", rowsAffected, maxConfidence)

	s.writeJSONResponse(w, map[string]interface{}{
		"success":       true,
		"message":       fmt.Sprintf("Классификация с уверенностью < %.2f сброшена", maxConfidence),
		"rows_affected": rowsAffected,
		"max_confidence": maxConfidence,
	}, http.StatusOK)
}

// handleMarkIncorrect помечает классификацию как неправильную
func (s *Server) handleMarkIncorrect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req MarkIncorrectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeJSONError(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	if req.NormalizedName == "" || req.Category == "" {
		s.writeJSONError(w, "normalized_name and category are required", http.StatusBadRequest)
		return
	}

	// Помечаем как неправильную и сбрасываем классификацию
	query := `UPDATE normalized_data 
		SET validation_status = 'incorrect',
		    validation_reason = ?,
		    kpved_code = NULL, kpved_name = NULL, kpved_confidence = 0.0
		WHERE normalized_name = ? AND category = ?`

	result, err := s.db.Exec(query, req.Reason, req.NormalizedName, req.Category)
	if err != nil {
		log.Printf("[MarkIncorrect] Error: %v", err)
		s.writeJSONError(w, fmt.Sprintf("Failed to mark as incorrect: %v", err), http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	log.Printf("[MarkIncorrect] Marked %d records as incorrect: %s / %s", rowsAffected, req.NormalizedName, req.Category)

	s.writeJSONResponse(w, map[string]interface{}{
		"success":       true,
		"message":       "Классификация помечена как неправильная и сброшена",
		"rows_affected": rowsAffected,
		"normalized_name": req.NormalizedName,
		"category": req.Category,
		"reason": req.Reason,
	}, http.StatusOK)
}

// handleMarkCorrect снимает пометку неправильной классификации
func (s *Server) handleMarkCorrect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		NormalizedName string `json:"normalized_name"`
		Category       string `json:"category"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeJSONError(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	if req.NormalizedName == "" || req.Category == "" {
		s.writeJSONError(w, "normalized_name and category are required", http.StatusBadRequest)
		return
	}

	query := `UPDATE normalized_data 
		SET validation_status = 'correct'
		WHERE normalized_name = ? AND category = ?`

	result, err := s.db.Exec(query, req.NormalizedName, req.Category)
	if err != nil {
		log.Printf("[MarkCorrect] Error: %v", err)
		s.writeJSONError(w, fmt.Sprintf("Failed to mark as correct: %v", err), http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	log.Printf("[MarkCorrect] Marked %d records as correct: %s / %s", rowsAffected, req.NormalizedName, req.Category)

	s.writeJSONResponse(w, map[string]interface{}{
		"success":       true,
		"message":       "Классификация помечена как правильная",
		"rows_affected": rowsAffected,
		"normalized_name": req.NormalizedName,
		"category": req.Category,
	}, http.StatusOK)
}

// handleKpvedWorkersStatus возвращает статус всех воркеров
func (s *Server) handleKpvedWorkersStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.kpvedCurrentTasksMutex.RLock()
	s.kpvedWorkersStopMutex.RLock()
	
	currentTasks := []map[string]interface{}{}
	for workerID, task := range s.kpvedCurrentTasks {
		if task != nil {
			currentTasks = append(currentTasks, map[string]interface{}{
				"worker_id":       workerID,
				"normalized_name": task.normalizedName,
				"category":        task.category,
				"merged_count":    task.mergedCount,
				"index":           task.index,
			})
		}
	}

	isRunning := len(currentTasks) > 0
	stopped := s.kpvedWorkersStopped

	s.kpvedWorkersStopMutex.RUnlock()
	s.kpvedCurrentTasksMutex.RUnlock()

	status := WorkersStatus{
		IsRunning:    isRunning,
		WorkersCount: len(currentTasks),
		Stopped:      stopped,
		CurrentTasks: currentTasks,
	}

	s.writeJSONResponse(w, status, http.StatusOK)
}

// handleKpvedWorkersStop останавливает все воркеры
func (s *Server) handleKpvedWorkersStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.kpvedWorkersStopMutex.Lock()
	s.kpvedWorkersStopped = true
	s.kpvedWorkersStopMutex.Unlock()

	log.Printf("[KpvedWorkersStop] Workers stop flag set to true")

	s.writeJSONResponse(w, map[string]interface{}{
		"success": true,
		"message": "Воркеры остановлены. Текущие задачи будут завершены, новые задачи не будут обрабатываться.",
		"stopped": true,
	}, http.StatusOK)
}

// handleKpvedWorkersResume возобновляет работу воркеров
func (s *Server) handleKpvedWorkersResume(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.kpvedWorkersStopMutex.Lock()
	s.kpvedWorkersStopped = false
	s.kpvedWorkersStopMutex.Unlock()

	log.Printf("[KpvedWorkersResume] Workers stop flag set to false")

	s.writeJSONResponse(w, map[string]interface{}{
		"success": true,
		"message": "Воркеры возобновлены",
		"stopped": false,
	}, http.StatusOK)
}

// handleKpvedStatsGeneral возвращает общую статистику классификации
func (s *Server) handleKpvedStatsGeneral(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	stats := KpvedStats{
		ByConfidence: make(map[string]int),
	}

	// Общее количество записей
	var totalRecords int
	err := s.db.QueryRow("SELECT COUNT(*) FROM normalized_data").Scan(&totalRecords)
	if err != nil {
		log.Printf("[KpvedStats] Error counting total: %v", err)
		s.writeJSONError(w, fmt.Sprintf("Failed to get stats: %v", err), http.StatusInternalServerError)
		return
	}
	stats.TotalRecords = totalRecords

	// Классифицированные записи
	var classified int
	err = s.db.QueryRow(`
		SELECT COUNT(*) FROM normalized_data 
		WHERE kpved_code IS NOT NULL AND kpved_code != '' AND TRIM(kpved_code) != ''
	`).Scan(&classified)
	if err == nil {
		stats.Classified = classified
		stats.NotClassified = totalRecords - classified
	}

	// Записи с низкой уверенностью
	var lowConfidence int
	err = s.db.QueryRow(`
		SELECT COUNT(*) FROM normalized_data 
		WHERE kpved_code IS NOT NULL AND kpved_code != '' 
		  AND kpved_confidence < 0.7
	`).Scan(&lowConfidence)
	if err == nil {
		stats.LowConfidence = lowConfidence
	}

	// Помеченные как неправильные
	var markedIncorrect int
	err = s.db.QueryRow(`
		SELECT COUNT(*) FROM normalized_data 
		WHERE validation_status = 'incorrect'
	`).Scan(&markedIncorrect)
	if err == nil {
		stats.MarkedIncorrect = markedIncorrect
	}

	// Статистика по уверенности
	var highConf, mediumConf, lowConf int
	s.db.QueryRow(`
		SELECT COUNT(*) FROM normalized_data 
		WHERE kpved_code IS NOT NULL AND kpved_code != '' 
		  AND kpved_confidence >= 0.9
	`).Scan(&highConf)
	s.db.QueryRow(`
		SELECT COUNT(*) FROM normalized_data 
		WHERE kpved_code IS NOT NULL AND kpved_code != '' 
		  AND kpved_confidence >= 0.7 AND kpved_confidence < 0.9
	`).Scan(&mediumConf)
	s.db.QueryRow(`
		SELECT COUNT(*) FROM normalized_data 
		WHERE kpved_code IS NOT NULL AND kpved_code != '' 
		  AND kpved_confidence < 0.7
	`).Scan(&lowConf)

	stats.ByConfidence["high"] = highConf
	stats.ByConfidence["medium"] = mediumConf
	stats.ByConfidence["low"] = lowConf

	s.writeJSONResponse(w, stats, http.StatusOK)
}

// handleKpvedStatsByCategory возвращает статистику по категориям
func (s *Server) handleKpvedStatsByCategory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	query := `
		SELECT 
			category,
			COUNT(*) as total,
			COUNT(CASE WHEN kpved_code IS NOT NULL AND kpved_code != '' THEN 1 END) as classified,
			COUNT(CASE WHEN kpved_code IS NULL OR kpved_code = '' THEN 1 END) as not_classified,
			COUNT(CASE WHEN kpved_code IS NOT NULL AND kpved_code != '' AND kpved_confidence < 0.7 THEN 1 END) as low_confidence,
			COUNT(CASE WHEN validation_status = 'incorrect' THEN 1 END) as marked_incorrect
		FROM normalized_data
		GROUP BY category
		ORDER BY total DESC
	`

	rows, err := s.db.Query(query)
	if err != nil {
		log.Printf("[KpvedStatsByCategory] Error: %v", err)
		s.writeJSONError(w, fmt.Sprintf("Failed to get stats: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	byCategory := make(map[string]CategoryStats)
	for rows.Next() {
		var category string
		var stats CategoryStats
		if err := rows.Scan(&category, &stats.Total, &stats.Classified, &stats.NotClassified, 
			&stats.LowConfidence, &stats.MarkedIncorrect); err != nil {
			continue
		}
		byCategory[category] = stats
	}

	response := map[string]interface{}{
		"by_category": byCategory,
	}

	s.writeJSONResponse(w, response, http.StatusOK)
}

// handleKpvedStatsIncorrect возвращает статистику неправильных классификаций
func (s *Server) handleKpvedStatsIncorrect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Получаем параметры запроса
	limit := 100 // По умолчанию
	if limitParam := r.URL.Query().Get("limit"); limitParam != "" {
		fmt.Sscanf(limitParam, "%d", &limit)
	}

	query := `
		SELECT DISTINCT normalized_name, category, kpved_code, kpved_name, 
		       kpved_confidence, validation_reason
		FROM normalized_data
		WHERE validation_status = 'incorrect'
		ORDER BY normalized_name, category
		LIMIT ?
	`

	rows, err := s.db.Query(query, limit)
	if err != nil {
		log.Printf("[KpvedStatsIncorrect] Error: %v", err)
		s.writeJSONError(w, fmt.Sprintf("Failed to get incorrect classifications: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	incorrectItems := []IncorrectClassificationItem{}
	for rows.Next() {
		var item IncorrectClassificationItem
		if err := rows.Scan(&item.NormalizedName, &item.Category, &item.KpvedCode, 
			&item.KpvedName, &item.Confidence, &item.Reason); err != nil {
			continue
		}
		incorrectItems = append(incorrectItems, item)
	}

	// Подсчитываем общее количество
	var totalIncorrect int
	s.db.QueryRow(`SELECT COUNT(DISTINCT normalized_name || '|' || category) 
		FROM normalized_data WHERE validation_status = 'incorrect'`).Scan(&totalIncorrect)

	response := map[string]interface{}{
		"total":           totalIncorrect,
		"shown":           len(incorrectItems),
		"incorrect_items": incorrectItems,
	}

	s.writeJSONResponse(w, response, http.StatusOK)
}

