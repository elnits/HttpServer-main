package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"

	"httpserver/normalization"
)

// StartNormalizationRequest запрос на начало нормализации
type StartNormalizationRequest struct {
	ItemID       int    `json:"item_id"`
	OriginalName string `json:"original_name"`
}

// NormalizationStageRequest запрос на применение стадии
type NormalizationStageRequest struct {
	SessionID int      `json:"session_id"`
	StageType string   `json:"stage_type"`
	Context   []string `json:"context,omitempty"`
	UseChat   bool     `json:"use_chat,omitempty"`
}

// RevertRequest запрос на откат
type RevertRequest struct {
	SessionID   int `json:"session_id"`
	TargetStage int `json:"target_stage"`
}

// handleStartNormalization начинает новую сессию нормализации
func (s *Server) handleStartNormalization(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req StartNormalizationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.ItemID == 0 || req.OriginalName == "" {
		s.writeJSONError(w, "item_id and original_name are required", http.StatusBadRequest)
		return
	}

	// Создаем компоненты для пайплайна
	patternDetector := normalization.NewPatternDetector()
	
	var aiIntegrator *normalization.PatternAIIntegrator
	apiKey := os.Getenv("ARLIAI_API_KEY")
	if apiKey != "" {
		aiNormalizer := normalization.NewAINormalizer(apiKey)
		aiIntegrator = normalization.NewPatternAIIntegrator(patternDetector, aiNormalizer)
	}

	// Создаем пайплайн
	pipeline := normalization.NewVersionedNormalizationPipeline(
		s.db,
		patternDetector,
		aiIntegrator,
	)

	// Начинаем сессию
	if err := pipeline.StartSession(req.ItemID, req.OriginalName); err != nil {
		s.writeJSONError(w, fmt.Sprintf("Failed to start session: %v", err), http.StatusInternalServerError)
		return
	}

	s.writeJSONResponse(w, map[string]interface{}{
		"session_id":   pipeline.GetSessionID(),
		"current_name": pipeline.GetCurrentName(),
		"original_name": req.OriginalName,
	}, http.StatusOK)
}

// handleApplyPatterns применяет алгоритмические паттерны
func (s *Server) handleApplyPatterns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req NormalizationStageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Получаем сессию
	session, err := s.db.GetNormalizationSession(req.SessionID)
	if err != nil {
		s.writeJSONError(w, "Session not found", http.StatusNotFound)
		return
	}

	// Создаем пайплайн и восстанавливаем состояние
	patternDetector := normalization.NewPatternDetector()
	var aiIntegrator *normalization.PatternAIIntegrator
	apiKey := os.Getenv("ARLIAI_API_KEY")
	if apiKey != "" {
		aiNormalizer := normalization.NewAINormalizer(apiKey)
		aiIntegrator = normalization.NewPatternAIIntegrator(patternDetector, aiNormalizer)
	}

	pipeline := normalization.NewVersionedNormalizationPipeline(
		s.db,
		patternDetector,
		aiIntegrator,
	)

	// Восстанавливаем сессию (упрощенный подход - создаем новый пайплайн с существующей сессией)
	// Для полной реализации нужно добавить метод RestoreSession в пайплайн
	// Пока используем прямое создание сессии
	if err := pipeline.StartSession(session.CatalogItemID, session.OriginalName); err != nil {
		s.writeJSONError(w, fmt.Sprintf("Failed to restore session: %v", err), http.StatusInternalServerError)
		return
	}

	// Применяем паттерны
	if err := pipeline.ApplyPatterns(); err != nil {
		s.writeJSONError(w, fmt.Sprintf("Failed to apply patterns: %v", err), http.StatusInternalServerError)
		return
	}

	history, _ := pipeline.GetHistory()

	s.writeJSONResponse(w, map[string]interface{}{
		"session_id":   pipeline.GetSessionID(),
		"current_name": pipeline.GetCurrentName(),
		"stage_count":  len(history),
	}, http.StatusOK)
}

// handleApplyAI применяет AI коррекцию
func (s *Server) handleApplyAI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		NormalizationStageRequest
		UseChat bool `json:"use_chat"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Получаем сессию
	session, err := s.db.GetNormalizationSession(req.SessionID)
	if err != nil {
		s.writeJSONError(w, "Session not found", http.StatusNotFound)
		return
	}

	// Создаем пайплайн
	patternDetector := normalization.NewPatternDetector()
	var aiIntegrator *normalization.PatternAIIntegrator
	apiKey := os.Getenv("ARLIAI_API_KEY")
	if apiKey == "" {
		s.writeJSONError(w, "ARLIAI_API_KEY not set", http.StatusBadRequest)
		return
	}

	aiNormalizer := normalization.NewAINormalizer(apiKey)
	aiIntegrator = normalization.NewPatternAIIntegrator(patternDetector, aiNormalizer)

	pipeline := normalization.NewVersionedNormalizationPipeline(
		s.db,
		patternDetector,
		aiIntegrator,
	)

	// Восстанавливаем сессию
	if err := pipeline.StartSession(session.CatalogItemID, session.OriginalName); err != nil {
		s.writeJSONError(w, fmt.Sprintf("Failed to restore session: %v", err), http.StatusInternalServerError)
		return
	}

	// Применяем AI коррекцию
	if err := pipeline.ApplyAICorrection(req.UseChat, req.Context...); err != nil {
		s.writeJSONError(w, fmt.Sprintf("Failed to apply AI correction: %v", err), http.StatusInternalServerError)
		return
	}

	history, _ := pipeline.GetHistory()
	lastStage := history[len(history)-1]

	s.writeJSONResponse(w, map[string]interface{}{
		"session_id":   pipeline.GetSessionID(),
		"current_name": pipeline.GetCurrentName(),
		"stage_count":  len(history),
		"confidence":   lastStage.Confidence,
	}, http.StatusOK)
}

// handleGetSessionHistory получает историю сессии
func (s *Server) handleGetSessionHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sessionIDStr := r.URL.Query().Get("session_id")
	if sessionIDStr == "" {
		s.writeJSONError(w, "session_id is required", http.StatusBadRequest)
		return
	}

	sessionID, err := strconv.Atoi(sessionIDStr)
	if err != nil {
		s.writeJSONError(w, "Invalid session_id", http.StatusBadRequest)
		return
	}

	// Получаем сессию
	session, err := s.db.GetNormalizationSession(sessionID)
	if err != nil {
		s.writeJSONError(w, "Session not found", http.StatusNotFound)
		return
	}

	// Получаем историю
	history, err := s.db.GetSessionHistory(sessionID)
	if err != nil {
		s.writeJSONError(w, fmt.Sprintf("Failed to get history: %v", err), http.StatusInternalServerError)
		return
	}

	s.writeJSONResponse(w, map[string]interface{}{
		"session": session,
		"history": history,
	}, http.StatusOK)
}

// handleRevertStage откатывает к указанной стадии
func (s *Server) handleRevertStage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req RevertRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Получаем сессию
	session, err := s.db.GetNormalizationSession(req.SessionID)
	if err != nil {
		s.writeJSONError(w, "Session not found", http.StatusNotFound)
		return
	}

	// Создаем пайплайн
	patternDetector := normalization.NewPatternDetector()
	var aiIntegrator *normalization.PatternAIIntegrator
	apiKey := os.Getenv("ARLIAI_API_KEY")
	if apiKey != "" {
		aiNormalizer := normalization.NewAINormalizer(apiKey)
		aiIntegrator = normalization.NewPatternAIIntegrator(patternDetector, aiNormalizer)
	}

	pipeline := normalization.NewVersionedNormalizationPipeline(
		s.db,
		patternDetector,
		aiIntegrator,
	)

	// Восстанавливаем сессию
	if err := pipeline.StartSession(session.CatalogItemID, session.OriginalName); err != nil {
		s.writeJSONError(w, fmt.Sprintf("Failed to restore session: %v", err), http.StatusInternalServerError)
		return
	}

	// Откатываем
	if err := pipeline.RevertToStage(req.TargetStage); err != nil {
		s.writeJSONError(w, fmt.Sprintf("Failed to revert: %v", err), http.StatusInternalServerError)
		return
	}

	s.writeJSONResponse(w, map[string]interface{}{
		"success":         true,
		"session_id":      req.SessionID,
		"reverted_to_stage": req.TargetStage,
		"current_name":    pipeline.GetCurrentName(),
	}, http.StatusOK)
}

