package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"httpserver/normalization"
)

// handlePatternDetect обнаруживает паттерны в названии
func (s *Server) handlePatternDetect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var request struct {
		Name string `json:"name"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		s.writeJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if request.Name == "" {
		s.writeJSONError(w, "Name is required", http.StatusBadRequest)
		return
	}

	// Создаем детектор паттернов
	detector := normalization.NewPatternDetector()

	// Обнаруживаем паттерны
	matches := detector.DetectPatterns(request.Name)

	// Применяем алгоритмические исправления
	algorithmicFix := detector.ApplyFixes(request.Name, matches)

	// Получаем сводку
	summary := detector.GetPatternSummary(matches)

	response := map[string]interface{}{
		"original_name":     request.Name,
		"patterns":          matches,
		"algorithmic_fix":   algorithmicFix,
		"summary":           summary,
		"patterns_count":    len(matches),
	}

	s.writeJSONResponse(w, response, http.StatusOK)
}

// handlePatternSuggest предлагает исправление с использованием паттернов и AI
func (s *Server) handlePatternSuggest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var request struct {
		Name string `json:"name"`
		UseAI bool  `json:"use_ai,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		s.writeJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if request.Name == "" {
		s.writeJSONError(w, "Name is required", http.StatusBadRequest)
		return
	}

	// Создаем детектор паттернов
	detector := normalization.NewPatternDetector()

	// Обнаруживаем паттерны
	matches := detector.DetectPatterns(request.Name)

	// Применяем алгоритмические исправления
	algorithmicFix := detector.ApplyFixes(request.Name, matches)

	response := map[string]interface{}{
		"original_name":    request.Name,
		"patterns":         matches,
		"algorithmic_fix":  algorithmicFix,
		"patterns_count":   len(matches),
	}

	// Если запрошено использование AI и API ключ доступен
	if request.UseAI {
		apiKey := os.Getenv("ARLIAI_API_KEY")
		if apiKey != "" {
			aiNormalizer := normalization.NewAINormalizer(apiKey)
			aiIntegrator := normalization.NewPatternAIIntegrator(detector, aiNormalizer)

			aiResult, err := aiIntegrator.SuggestCorrectionWithAI(request.Name)
			if err == nil {
				response["ai_suggested_fix"] = aiResult.AISuggestedFix
				response["final_suggestion"] = aiResult.FinalSuggestion
				response["confidence"] = aiResult.Confidence
				response["reasoning"] = aiResult.Reasoning
				response["requires_review"] = aiResult.RequiresReview
			} else {
				response["ai_error"] = err.Error()
				response["final_suggestion"] = algorithmicFix
			}
		} else {
			response["ai_error"] = "ARLIAI_API_KEY not set"
			response["final_suggestion"] = algorithmicFix
		}
	} else {
		response["final_suggestion"] = algorithmicFix
	}

	s.writeJSONResponse(w, response, http.StatusOK)
}

// handlePatternTestBatch тестирует паттерны на выборке данных из базы
func (s *Server) handlePatternTestBatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var request struct {
		Limit  int  `json:"limit,omitempty"`
		UseAI  bool `json:"use_ai,omitempty"`
		Table  string `json:"table,omitempty"`
		Column string `json:"column,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		// Если тело пустое, используем дефолтные значения
		request.Limit = 50
		request.UseAI = false
		request.Table = "catalog_items"
		request.Column = "name"
	}

	if request.Limit <= 0 {
		request.Limit = 50
	}
	if request.Limit > 500 {
		request.Limit = 500 // Ограничение для безопасности
	}

	if request.Table == "" {
		request.Table = "catalog_items"
	}
	if request.Column == "" {
		request.Column = "name"
	}

	// Создаем детектор паттернов
	detector := normalization.NewPatternDetector()

	// Инициализируем AI интегратор если нужно
	var aiIntegrator *normalization.PatternAIIntegrator
	if request.UseAI {
		apiKey := os.Getenv("ARLIAI_API_KEY")
		if apiKey != "" {
			aiNormalizer := normalization.NewAINormalizer(apiKey)
			aiIntegrator = normalization.NewPatternAIIntegrator(detector, aiNormalizer)
		}
	}

	// Получаем названия из базы
	query := fmt.Sprintf(`
		SELECT DISTINCT %s 
		FROM %s 
		WHERE %s IS NOT NULL AND %s != ''
		LIMIT ?
	`, request.Column, request.Table, request.Column, request.Column)

	rows, err := s.db.Query(query, request.Limit)
	if err != nil {
		s.writeJSONError(w, fmt.Sprintf("Database query error: %v", err), http.StatusInternalServerError)
		return
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

	// Анализируем каждое название
	results := make([]map[string]interface{}, 0, len(names))

	for _, name := range names {
		// Обнаруживаем паттерны
		matches := detector.DetectPatterns(name)

		result := map[string]interface{}{
			"original_name":    name,
			"patterns_found":   len(matches),
			"patterns":         matches,
		}

		// Применяем алгоритмические исправления
		algorithmicFix := detector.ApplyFixes(name, matches)
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
				result["final_suggestion"] = algorithmicFix
			}
		} else {
			result["final_suggestion"] = algorithmicFix
		}

		results = append(results, result)
	}

	// Вычисляем статистику
	stats := calculatePatternStats(results)

	response := map[string]interface{}{
		"total_analyzed": len(results),
		"results":        results,
		"statistics":     stats,
	}

	s.writeJSONResponse(w, response, http.StatusOK)
}

// calculatePatternStats вычисляет статистику по результатам анализа паттернов
func calculatePatternStats(results []map[string]interface{}) map[string]interface{} {
	stats := make(map[string]interface{})

	totalPatterns := 0
	patternTypes := make(map[string]int)
	severityCount := make(map[string]int)
	autoFixableCount := 0
	itemsWithPatterns := 0
	itemsRequiringReview := 0

	for _, result := range results {
		patternsCount := 0
		if count, ok := result["patterns_found"].(int); ok {
			patternsCount = count
		}

		if patternsCount > 0 {
			itemsWithPatterns++
		}

		if patterns, ok := result["patterns"].([]normalization.PatternMatch); ok {
			totalPatterns += len(patterns)
			for _, match := range patterns {
				patternTypes[string(match.Type)]++
				severityCount[match.Severity]++
				if match.AutoFixable {
					autoFixableCount++
				}
			}
		}

		if requiresReview, ok := result["requires_review"].(bool); ok && requiresReview {
			itemsRequiringReview++
		}
	}

	stats["total_patterns"] = totalPatterns
	stats["items_with_patterns"] = itemsWithPatterns
	stats["items_requiring_review"] = itemsRequiringReview
	stats["auto_fixable_patterns"] = autoFixableCount
	stats["patterns_by_type"] = patternTypes
	stats["patterns_by_severity"] = severityCount

	if len(results) > 0 {
		stats["avg_patterns_per_item"] = float64(totalPatterns) / float64(len(results))
		stats["items_with_patterns_percent"] = float64(itemsWithPatterns) / float64(len(results)) * 100
	}

	return stats
}

