package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"httpserver/database"
	"httpserver/quality"
)

// ============================================================================
// DQAS (Data Quality Assessment System) Handlers
// ============================================================================

// handleQualityItemDetail возвращает детальную информацию о качестве конкретной записи
func (s *Server) handleQualityItemDetail(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Извлекаем ID из URL (например, /api/quality/item/123)
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/quality/item/"), "/")
	if len(pathParts) == 0 || pathParts[0] == "" {
		s.writeJSONError(w, "Item ID is required", http.StatusBadRequest)
		return
	}

	itemID, err := strconv.Atoi(pathParts[0])
	if err != nil {
		s.writeJSONError(w, "Invalid item ID", http.StatusBadRequest)
		return
	}

	// Получаем последнюю оценку качества
	assessment, err := s.normalizedDB.GetQualityAssessment(itemID)
	if err != nil {
		log.Printf("Error getting quality assessment for item %d: %v", itemID, err)
		s.writeJSONError(w, fmt.Sprintf("Failed to get quality assessment: %v", err), http.StatusInternalServerError)
		return
	}

	// Получаем violations для этой записи
	violations, _, err := s.normalizedDB.GetViolations(map[string]interface{}{
		"normalized_item_id": itemID,
	}, 100, 0)
	if err != nil {
		log.Printf("Error getting violations for item %d: %v", itemID, err)
		// Не возвращаем ошибку, просто пустой массив
		violations = []database.QualityViolation{}
	}

	// Получаем suggestions для этой записи
	suggestions, _, err := s.normalizedDB.GetSuggestions(map[string]interface{}{
		"normalized_item_id": itemID,
		"applied":            false,
	}, 100, 0)
	if err != nil {
		log.Printf("Error getting suggestions for item %d: %v", itemID, err)
		suggestions = []database.QualitySuggestion{}
	}

	response := map[string]interface{}{
		"assessment":  assessment,
		"violations":  violations,
		"suggestions": suggestions,
	}

	s.writeJSONResponse(w, response, http.StatusOK)
}

// handleQualityViolations возвращает список нарушений правил качества
func (s *Server) handleQualityViolations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Получаем параметр database из query
	databasePath := r.URL.Query().Get("database")
	if databasePath == "" {
		// Если не указан, используем normalizedDB по умолчанию
		databasePath = s.currentNormalizedDBPath
	}

	// Открываем нужную БД
	var db *database.DB
	var err error
	if databasePath != "" && databasePath != s.currentNormalizedDBPath {
		db, err = database.NewDB(databasePath)
		if err != nil {
			log.Printf("Error opening database %s: %v", databasePath, err)
			s.writeJSONError(w, fmt.Sprintf("Failed to open database: %v", err), http.StatusInternalServerError)
			return
		}
		defer db.Close()
	} else {
		db = s.normalizedDB
	}

	// Параметры фильтрации
	filters := make(map[string]interface{})

	if severity := r.URL.Query().Get("severity"); severity != "" {
		filters["severity"] = severity
	}

	if category := r.URL.Query().Get("category"); category != "" {
		filters["category"] = category
	}

	// Pagination
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 50
	}

	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if offset < 0 {
		offset = 0
	}

	violations, total, err := db.GetViolations(filters, limit, offset)
	if err != nil {
		log.Printf("Error getting violations: %v", err)
		s.writeJSONError(w, fmt.Sprintf("Failed to get violations: %v", err), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"violations": violations,
		"total":      total,
		"limit":      limit,
		"offset":     offset,
	}

	s.writeJSONResponse(w, response, http.StatusOK)
}

// handleQualityViolationDetail обрабатывает действия с конкретным нарушением
func (s *Server) handleQualityViolationDetail(w http.ResponseWriter, r *http.Request) {
	// Извлекаем ID из URL
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/quality/violations/"), "/")
	if len(pathParts) == 0 || pathParts[0] == "" {
		s.writeJSONError(w, "Violation ID is required", http.StatusBadRequest)
		return
	}

	violationID, err := strconv.Atoi(pathParts[0])
	if err != nil {
		s.writeJSONError(w, "Invalid violation ID", http.StatusBadRequest)
		return
	}

	// POST - разрешить нарушение
	if r.Method == http.MethodPost {
		var reqBody struct {
			ResolvedBy string `json:"resolved_by"`
		}

		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			s.writeJSONError(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		if err := s.normalizedDB.ResolveViolation(violationID, reqBody.ResolvedBy); err != nil {
			log.Printf("Error resolving violation %d: %v", violationID, err)
			s.writeJSONError(w, fmt.Sprintf("Failed to resolve violation: %v", err), http.StatusInternalServerError)
			return
		}

		s.writeJSONResponse(w, map[string]interface{}{
			"success": true,
			"message": "Violation resolved",
		}, http.StatusOK)
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

// handleQualitySuggestions возвращает список предложений по улучшению
func (s *Server) handleQualitySuggestions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Получаем параметр database из query
	databasePath := r.URL.Query().Get("database")
	if databasePath == "" {
		// Если не указан, используем normalizedDB по умолчанию
		databasePath = s.currentNormalizedDBPath
	}

	// Открываем нужную БД
	var db *database.DB
	var err error
	if databasePath != "" && databasePath != s.currentNormalizedDBPath {
		db, err = database.NewDB(databasePath)
		if err != nil {
			log.Printf("Error opening database %s: %v", databasePath, err)
			s.writeJSONError(w, fmt.Sprintf("Failed to open database: %v", err), http.StatusInternalServerError)
			return
		}
		defer db.Close()
	} else {
		db = s.normalizedDB
	}

	// Параметры фильтрации
	filters := make(map[string]interface{})

	if priority := r.URL.Query().Get("priority"); priority != "" {
		filters["priority"] = priority
	}

	if autoApplyable := r.URL.Query().Get("auto_applyable"); autoApplyable == "true" {
		filters["auto_applyable"] = true
	}

	if applied := r.URL.Query().Get("applied"); applied == "false" {
		filters["applied"] = false
	}

	// Pagination
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 50
	}

	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if offset < 0 {
		offset = 0
	}

	suggestions, total, err := db.GetSuggestions(filters, limit, offset)
	if err != nil {
		log.Printf("Error getting suggestions: %v", err)
		s.writeJSONError(w, fmt.Sprintf("Failed to get suggestions: %v", err), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"suggestions": suggestions,
		"total":       total,
		"limit":       limit,
		"offset":      offset,
	}

	s.writeJSONResponse(w, response, http.StatusOK)
}

// handleQualityMetrics возвращает метрики качества для проекта
func (s *Server) handleQualityMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	projectID, err := strconv.Atoi(r.URL.Query().Get("project_id"))
	if err != nil {
		s.writeJSONError(w, "Invalid project ID", http.StatusBadRequest)
		return
	}

	period := r.URL.Query().Get("period")
	if period == "" {
		period = "month" // default period
	}

	metrics, err := s.serviceDB.GetQualityMetricsForProject(projectID, period)
	if err != nil {
		log.Printf("Error getting quality metrics: %v", err)
		s.writeJSONError(w, "Failed to get quality metrics", http.StatusInternalServerError)
		return
	}

	s.writeJSONResponse(w, metrics, http.StatusOK)
}

// handleCompareProjectsQuality сравнивает качество между проектами
func (s *Server) handleCompareProjectsQuality(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var projectIDs []int
	if err := json.NewDecoder(r.Body).Decode(&projectIDs); err != nil {
		s.writeJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if len(projectIDs) < 2 {
		s.writeJSONError(w, "At least two project IDs required", http.StatusBadRequest)
		return
	}

	comparison, err := s.serviceDB.CompareProjectsQuality(projectIDs)
	if err != nil {
		log.Printf("Error comparing projects quality: %v", err)
		s.writeJSONError(w, "Failed to compare projects", http.StatusInternalServerError)
		return
	}

	s.writeJSONResponse(w, comparison, http.StatusOK)
}

// handleQualitySuggestionAction обрабатывает действия с предложениями
func (s *Server) handleQualitySuggestionAction(w http.ResponseWriter, r *http.Request) {
	// Извлекаем ID из URL
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/quality/suggestions/"), "/")
	if len(pathParts) == 0 || pathParts[0] == "" {
		s.writeJSONError(w, "Suggestion ID is required", http.StatusBadRequest)
		return
	}

	suggestionID, err := strconv.Atoi(pathParts[0])
	if err != nil {
		s.writeJSONError(w, "Invalid suggestion ID", http.StatusBadRequest)
		return
	}

	// POST - применить предложение
	if r.Method == http.MethodPost {
		// Проверяем action
		action := ""
		if len(pathParts) > 1 {
			action = pathParts[1]
		}

		if action == "apply" {
			if err := s.normalizedDB.ApplySuggestion(suggestionID); err != nil {
				log.Printf("Error applying suggestion %d: %v", suggestionID, err)
				s.writeJSONError(w, fmt.Sprintf("Failed to apply suggestion: %v", err), http.StatusInternalServerError)
				return
			}

			s.writeJSONResponse(w, map[string]interface{}{
				"success": true,
				"message": "Suggestion applied",
			}, http.StatusOK)
			return
		}

		s.writeJSONError(w, "Unknown action", http.StatusBadRequest)
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

// handleQualityDuplicates возвращает список групп дубликатов
func (s *Server) handleQualityDuplicates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Получаем параметр database из query
	databasePath := r.URL.Query().Get("database")
	if databasePath == "" {
		// Если не указан, используем normalizedDB по умолчанию
		databasePath = s.currentNormalizedDBPath
	}

	// Открываем нужную БД
	var db *database.DB
	var err error
	if databasePath != "" && databasePath != s.currentNormalizedDBPath {
		db, err = database.NewDB(databasePath)
		if err != nil {
			log.Printf("Error opening database %s: %v", databasePath, err)
			s.writeJSONError(w, fmt.Sprintf("Failed to open database: %v", err), http.StatusInternalServerError)
			return
		}
		defer db.Close()
	} else {
		db = s.normalizedDB
	}

	// Параметры фильтрации
	onlyUnmerged := r.URL.Query().Get("unmerged") == "true"

	// Pagination
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 50
	}

	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if offset < 0 {
		offset = 0
	}

	groups, total, err := db.GetDuplicateGroups(onlyUnmerged, limit, offset)
	if err != nil {
		log.Printf("Error getting duplicate groups: %v", err)
		s.writeJSONError(w, fmt.Sprintf("Failed to get duplicate groups: %v", err), http.StatusInternalServerError)
		return
	}

	// Обогащаем группы полными данными элементов
	enrichedGroups := make([]map[string]interface{}, len(groups))
	for i, group := range groups {
		enrichedGroup := map[string]interface{}{
			"id":                group.ID,
			"group_hash":        group.GroupHash,
			"duplicate_type":    group.DuplicateType,
			"similarity_score":  group.SimilarityScore,
			"item_ids":          group.ItemIDs,
			"suggested_master_id": group.SuggestedMasterID,
			"confidence":        group.Confidence,
			"reason":            group.Reason,
			"merged":            group.Merged,
			"merged_at":         group.MergedAt,
			"created_at":        group.CreatedAt,
			"updated_at":        group.UpdatedAt,
			"item_count":        len(group.ItemIDs),
		}

		// Загружаем полные данные элементов
		if len(group.ItemIDs) > 0 {
			items := make([]map[string]interface{}, 0)
			// Формируем IN запрос для получения всех элементов за раз
			placeholders := make([]string, len(group.ItemIDs))
			args := make([]interface{}, len(group.ItemIDs))
			for i, id := range group.ItemIDs {
				placeholders[i] = "?"
				args[i] = id
			}
			
			// Пытаемся найти элементы в разных таблицах
			// Сначала пробуем normalized_data
			query := fmt.Sprintf(`
				SELECT id, 
					COALESCE(code, '') as code, 
					COALESCE(normalized_name, '') as normalized_name, 
					COALESCE(category, '') as category, 
					COALESCE(kpved_code, '') as kpved_code, 
					COALESCE(processing_level, 'basic') as processing_level, 
					COALESCE(merged_count, 0) as merged_count
				FROM normalized_data
				WHERE id IN (%s)
			`, strings.Join(placeholders, ","))
			
			rows, err := db.Query(query, args...)
			if err != nil {
				// Если normalized_data не существует, пробуем nomenclature_items
				query = fmt.Sprintf(`
					SELECT id, 
						COALESCE(nomenclature_code, '') as code, 
						COALESCE(nomenclature_name, '') as normalized_name, 
						COALESCE(category, '') as category, 
						COALESCE(kpved_code, '') as kpved_code, 
						COALESCE(processing_level, 'basic') as processing_level, 
						0 as merged_count
					FROM nomenclature_items
					WHERE id IN (%s)
				`, strings.Join(placeholders, ","))
				rows, err = db.Query(query, args...)
			}
			if err != nil {
				// Если и nomenclature_items не существует, пробуем catalog_items
				query = fmt.Sprintf(`
					SELECT id, 
						COALESCE(code, '') as code, 
						COALESCE(name, '') as normalized_name, 
						COALESCE(category, '') as category, 
						COALESCE(kpved_code, '') as kpved_code, 
						COALESCE(processing_level, 'basic') as processing_level, 
						0 as merged_count
					FROM catalog_items
					WHERE id IN (%s)
				`, strings.Join(placeholders, ","))
				rows, err = db.Query(query, args...)
			}
			if err == nil {
				defer rows.Close()
				for rows.Next() {
					var id, mergedCount int
					var code, normalizedName, category, kpvedCode, processingLevel sql.NullString
					
					if err := rows.Scan(&id, &code, &normalizedName, &category, &kpvedCode, &processingLevel, &mergedCount); err == nil {
						items = append(items, map[string]interface{}{
							"id":               id,
							"code":             getStringValue(code),
							"normalized_name":  getStringValue(normalizedName),
							"category":         getStringValue(category),
							"kpved_code":       getStringValue(kpvedCode),
							"quality_score":    0.0, // Поле отсутствует в таблице normalized_data
							"processing_level": getStringValue(processingLevel),
							"merged_count":     mergedCount,
						})
					}
				}
			} else {
				// Если не удалось найти элементы ни в одной таблице, логируем ошибку
				log.Printf("Warning: Could not find items in any table for group %d: %v", group.ID, err)
			}
			enrichedGroup["items"] = items
		} else {
			enrichedGroup["items"] = []interface{}{}
		}

		enrichedGroups[i] = enrichedGroup
	}

	response := map[string]interface{}{
		"groups": enrichedGroups,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	}

	s.writeJSONResponse(w, response, http.StatusOK)
}

// getStringValue извлекает строковое значение из sql.NullString
func getStringValue(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}

// handleQualityDuplicateAction обрабатывает действия с группами дубликатов
func (s *Server) handleQualityDuplicateAction(w http.ResponseWriter, r *http.Request) {
	// Извлекаем ID из URL
	pathParts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/quality/duplicates/"), "/")
	if len(pathParts) == 0 || pathParts[0] == "" {
		s.writeJSONError(w, "Duplicate group ID is required", http.StatusBadRequest)
		return
	}

	groupID, err := strconv.Atoi(pathParts[0])
	if err != nil {
		s.writeJSONError(w, "Invalid group ID", http.StatusBadRequest)
		return
	}

	// POST - действия с группой
	if r.Method == http.MethodPost {
		action := ""
		if len(pathParts) > 1 {
			action = pathParts[1]
		}

		if action == "merge" {
			if err := s.normalizedDB.MarkDuplicateGroupMerged(groupID); err != nil {
				log.Printf("Error merging duplicate group %d: %v", groupID, err)
				s.writeJSONError(w, fmt.Sprintf("Failed to merge duplicate group: %v", err), http.StatusInternalServerError)
				return
			}

			s.writeJSONResponse(w, map[string]interface{}{
				"success": true,
				"message": "Duplicate group marked as merged",
			}, http.StatusOK)
			return
		}

		s.writeJSONError(w, "Unknown action", http.StatusBadRequest)
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

// handleQualityAssess запускает оценку качества для всех записей или указанной записи
func (s *Server) handleQualityAssess(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var reqBody struct {
		ItemID int `json:"item_id,omitempty"` // Если указан - оценить только эту запись
	}

	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		s.writeJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Запуск оценки качества реализован через handleQualityAnalysis
	// Этот endpoint используется для ручного запуска оценки
	// В данный момент оценка запускается автоматически при загрузке данных
	
	s.writeJSONResponse(w, map[string]interface{}{
		"success": true,
		"message": "Quality assessment is triggered automatically during data upload. Use /api/quality/analysis/{upload_uuid} for manual assessment.",
		"item_id": reqBody.ItemID,
	}, http.StatusOK)
}

// handleQualityAnalyze запускает анализ качества для указанной таблицы
func (s *Server) handleQualityAnalyze(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var reqBody struct {
		Database   string `json:"database"`
		Table      string `json:"table"`
		CodeColumn string `json:"code_column"`
		NameColumn string `json:"name_column"`
	}

	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		s.writeJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Проверяем, не выполняется ли уже анализ
	s.qualityAnalysisMutex.Lock()
	if s.qualityAnalysisRunning {
		s.qualityAnalysisMutex.Unlock()
		s.writeJSONError(w, "Analysis is already running", http.StatusConflict)
		return
	}
	s.qualityAnalysisRunning = true
	s.qualityAnalysisStatus = QualityAnalysisStatus{
		IsRunning:      true,
		Progress:       0,
		Processed:      0,
		Total:          0,
		CurrentStep:    "initializing",
		DuplicatesFound: 0,
		ViolationsFound: 0,
		SuggestionsFound: 0,
	}
	s.qualityAnalysisMutex.Unlock()

	// Определяем колонки по умолчанию если не указаны
	codeColumn := reqBody.CodeColumn
	nameColumn := reqBody.NameColumn

	if codeColumn == "" {
		switch reqBody.Table {
		case "normalized_data":
			codeColumn = "code"
		case "nomenclature_items":
			codeColumn = "nomenclature_code"
		case "catalog_items":
			codeColumn = "code"
		default:
			codeColumn = "code"
		}
	}

	if nameColumn == "" {
		switch reqBody.Table {
		case "normalized_data":
			nameColumn = "normalized_name"
		case "nomenclature_items":
			nameColumn = "nomenclature_name"
		case "catalog_items":
			nameColumn = "name"
		default:
			nameColumn = "name"
		}
	}

	// Открываем базу данных
	db, err := database.NewDB(reqBody.Database)
	if err != nil {
		s.qualityAnalysisMutex.Lock()
		s.qualityAnalysisRunning = false
		s.qualityAnalysisStatus.Error = err.Error()
		s.qualityAnalysisMutex.Unlock()
		s.writeJSONError(w, fmt.Sprintf("Failed to open database: %v", err), http.StatusInternalServerError)
		return
	}

	// Запускаем анализ в фоновой горутине
	go s.runQualityAnalysis(db, reqBody.Table, codeColumn, nameColumn)

	s.writeJSONResponse(w, map[string]interface{}{
		"success": true,
		"message": "Quality analysis started",
		"table":   reqBody.Table,
	}, http.StatusOK)
}

// runQualityAnalysis выполняет анализ качества в фоновом режиме
func (s *Server) runQualityAnalysis(db *database.DB, tableName, codeColumn, nameColumn string) {
	defer db.Close()
	defer func() {
		s.qualityAnalysisMutex.Lock()
		s.qualityAnalysisRunning = false
		if s.qualityAnalysisStatus.Error == "" {
			s.qualityAnalysisStatus.CurrentStep = "completed"
			s.qualityAnalysisStatus.Progress = 100
		}
		s.qualityAnalysisMutex.Unlock()
	}()

	analyzer := quality.NewTableAnalyzer(db)
	batchSize := 1000

	// 1. Анализ дубликатов
	s.qualityAnalysisMutex.Lock()
	s.qualityAnalysisStatus.CurrentStep = "duplicates"
	s.qualityAnalysisMutex.Unlock()

	duplicatesCount, err := analyzer.AnalyzeTableForDuplicates(
		tableName, codeColumn, nameColumn, batchSize,
		func(processed, total int) {
			s.qualityAnalysisMutex.Lock()
			s.qualityAnalysisStatus.Processed = processed
			s.qualityAnalysisStatus.Total = total
			if total > 0 {
				s.qualityAnalysisStatus.Progress = float64(processed) / float64(total) * 33.33
			}
			s.qualityAnalysisMutex.Unlock()
		},
	)

	if err != nil {
		s.qualityAnalysisMutex.Lock()
		s.qualityAnalysisStatus.Error = fmt.Sprintf("Duplicate analysis failed: %v", err)
		s.qualityAnalysisMutex.Unlock()
		return
	}

	s.qualityAnalysisMutex.Lock()
	s.qualityAnalysisStatus.DuplicatesFound = duplicatesCount
	s.qualityAnalysisMutex.Unlock()

	// 2. Анализ нарушений
	s.qualityAnalysisMutex.Lock()
	s.qualityAnalysisStatus.CurrentStep = "violations"
	s.qualityAnalysisStatus.Processed = 0
	s.qualityAnalysisStatus.Total = 0
	s.qualityAnalysisMutex.Unlock()

	violationsCount, err := analyzer.AnalyzeTableForViolations(
		tableName, codeColumn, nameColumn, batchSize,
		func(processed, total int) {
			s.qualityAnalysisMutex.Lock()
			s.qualityAnalysisStatus.Processed = processed
			s.qualityAnalysisStatus.Total = total
			if total > 0 {
				s.qualityAnalysisStatus.Progress = 33.33 + float64(processed)/float64(total)*33.33
			}
			s.qualityAnalysisMutex.Unlock()
		},
	)

	if err != nil {
		s.qualityAnalysisMutex.Lock()
		s.qualityAnalysisStatus.Error = fmt.Sprintf("Violations analysis failed: %v", err)
		s.qualityAnalysisMutex.Unlock()
		return
	}

	s.qualityAnalysisMutex.Lock()
	s.qualityAnalysisStatus.ViolationsFound = violationsCount
	s.qualityAnalysisMutex.Unlock()

	// 3. Анализ предложений
	s.qualityAnalysisMutex.Lock()
	s.qualityAnalysisStatus.CurrentStep = "suggestions"
	s.qualityAnalysisStatus.Processed = 0
	s.qualityAnalysisStatus.Total = 0
	s.qualityAnalysisMutex.Unlock()

	suggestionsCount, err := analyzer.AnalyzeTableForSuggestions(
		tableName, codeColumn, nameColumn, batchSize,
		func(processed, total int) {
			s.qualityAnalysisMutex.Lock()
			s.qualityAnalysisStatus.Processed = processed
			s.qualityAnalysisStatus.Total = total
			if total > 0 {
				s.qualityAnalysisStatus.Progress = 66.66 + float64(processed)/float64(total)*33.34
			}
			s.qualityAnalysisMutex.Unlock()
		},
	)

	if err != nil {
		s.qualityAnalysisMutex.Lock()
		s.qualityAnalysisStatus.Error = fmt.Sprintf("Suggestions analysis failed: %v", err)
		s.qualityAnalysisMutex.Unlock()
		return
	}

	s.qualityAnalysisMutex.Lock()
	s.qualityAnalysisStatus.SuggestionsFound = suggestionsCount
	s.qualityAnalysisMutex.Unlock()
}

// handleQualityAnalyzeStatus возвращает статус анализа качества
func (s *Server) handleQualityAnalyzeStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.qualityAnalysisMutex.RLock()
	status := s.qualityAnalysisStatus
	s.qualityAnalysisMutex.RUnlock()

	s.writeJSONResponse(w, status, http.StatusOK)
}