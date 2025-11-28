package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"httpserver/classification"
	"httpserver/database"
	"httpserver/normalization"
)

// ClassificationRequest запрос на классификацию
type ClassificationRequest struct {
	SessionID  int                    `json:"session_id"`
	StrategyID string                 `json:"strategy_id"`
	Context    map[string]interface{} `json:"context,omitempty"`
}

// StrategyConfigRequest запрос на конфигурацию стратегии
type StrategyConfigRequest struct {
	ClientID    int                          `json:"client_id"`
	MaxDepth    int                          `json:"max_depth"`
	Priority    []string                     `json:"priority"`
	Rules       []classification.FoldingRule `json:"rules"`
	Name        string                       `json:"name"`
	Description string                       `json:"description"`
}

// handleClassifyItem классифицирует товар
func (s *Server) handleClassifyItem(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ClassificationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.SessionID == 0 {
		s.writeJSONError(w, "session_id is required", http.StatusBadRequest)
		return
	}

	if req.StrategyID == "" {
		req.StrategyID = "top_priority" // Дефолтная стратегия
	}

	// Получаем сессию
	session, err := s.db.GetNormalizationSession(req.SessionID)
	if err != nil {
		s.writeJSONError(w, "Session not found", http.StatusNotFound)
		return
	}

	// Создаем компоненты
	patternDetector := normalization.NewPatternDetector()
	var aiIntegrator *normalization.PatternAIIntegrator
	apiKey := os.Getenv("ARLIAI_API_KEY")
	if apiKey == "" {
		s.writeJSONError(w, "ARLIAI_API_KEY not set", http.StatusBadRequest)
		return
	}

	// Получаем модель из WorkerConfigManager
	model := s.getModelFromConfig()

	aiNormalizer := normalization.NewAINormalizer(apiKey, model)
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

	// Создаем AI классификатор
	aiClassifier := classification.NewAIClassifier(apiKey, model)

	// Создаем менеджер стратегий
	strategyManager := classification.NewStrategyManager()

	// Создаем стадию классификации
	classificationStage := normalization.NewClassificationStage(aiClassifier, strategyManager)

	// Применяем классификацию
	if err := classificationStage.Process(pipeline, req.StrategyID); err != nil {
		s.writeJSONError(w, fmt.Sprintf("Classification failed: %v", err), http.StatusInternalServerError)
		return
	}

	// Получаем результаты из метаданных
	categoryFolded := pipeline.GetMetadata("category_folded")
	categoryOriginal := pipeline.GetMetadata("category_original")
	confidence := pipeline.GetMetadata("classification_confidence")

	// Преобразуем в нужный формат
	var foldedLevels []string
	if folded, ok := categoryFolded.([]string); ok {
		foldedLevels = folded
	}

	var originalLevels []string
	if orig, ok := categoryOriginal.([]string); ok {
		originalLevels = orig
	}

	var conf float64
	if c, ok := confidence.(float64); ok {
		conf = c
	}

	// Формируем уровни для ответа
	levels := make(map[string]string)
	for i, level := range foldedLevels {
		if i < 5 { // Максимум 5 уровней
			levels[fmt.Sprintf("level%d", i+1)] = level
		}
	}

	s.writeJSONResponse(w, map[string]interface{}{
		"session_id":        pipeline.GetSessionID(),
		"original_category": originalLevels,
		"folded_category":   foldedLevels,
		"levels":            levels,
		"confidence":        conf,
		"strategy":          req.StrategyID,
	}, http.StatusOK)
}

// handleApplyCategorization применяет этап классификации к сессии нормализации
func (s *Server) handleApplyCategorization(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ClassificationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.SessionID == 0 {
		s.writeJSONError(w, "session_id is required", http.StatusBadRequest)
		return
	}

	// Получаем сессию
	session, err := s.db.GetNormalizationSession(req.SessionID)
	if err != nil {
		s.writeJSONError(w, "Session not found", http.StatusNotFound)
		return
	}

	// Создаем компоненты
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

	// Получаем модель из WorkerConfigManager
	model := s.getModelFromConfig()
	aiClassifier := classification.NewAIClassifier(apiKey, model)

	// Создаем менеджер стратегий
	strategyManager := classification.NewStrategyManager()

	// Создаем стадию классификации
	classificationStage := normalization.NewClassificationStage(aiClassifier, strategyManager)

	// Применяем классификацию
	strategyID := "top_priority"
	if req.StrategyID != "" {
		strategyID = req.StrategyID
	}

	if err := classificationStage.Process(pipeline, strategyID); err != nil {
		s.writeJSONError(w, fmt.Sprintf("Classification failed: %v", err), http.StatusInternalServerError)
		return
	}

	// Получаем результаты из метаданных
	categoryFolded := pipeline.GetMetadata("category_folded")
	categoryOriginal := pipeline.GetMetadata("category_original")
	confidence := pipeline.GetMetadata("classification_confidence")

	// Преобразуем в нужный формат
	var foldedLevels []string
	if folded, ok := categoryFolded.([]string); ok {
		foldedLevels = folded
	}

	var originalLevels []string
	if orig, ok := categoryOriginal.([]string); ok {
		originalLevels = orig
	}

	var conf float64
	if c, ok := confidence.(float64); ok {
		conf = c
	}

	// Формируем уровни для ответа
	levels := make(map[string]string)
	for i, level := range foldedLevels {
		if i < 5 { // Максимум 5 уровней
			levels[fmt.Sprintf("level%d", i+1)] = level
		}
	}

	s.writeJSONResponse(w, map[string]interface{}{
		"session_id":        pipeline.GetSessionID(),
		"original_category": originalLevels,
		"folded_category":   foldedLevels,
		"levels":            levels,
		"confidence":        conf,
		"strategy":          strategyID,
		"stage_applied":     "categorization",
	}, http.StatusOK)
}

// handleConfigureStrategy настраивает стратегию свертки
func (s *Server) handleConfigureStrategy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req StrategyConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.MaxDepth <= 0 {
		req.MaxDepth = 2
	}

	// Создаем стратегию
	strategy := classification.FoldingStrategyConfig{
		Name:        req.Name,
		Description: req.Description,
		MaxDepth:    req.MaxDepth,
		Priority:    req.Priority,
		Rules:       req.Rules,
	}

	// Генерируем ID
	strategy.ID = fmt.Sprintf("custom_%d_%d", req.ClientID, len(req.Priority))

	// Сохраняем стратегию (можно добавить в БД, пока используем только в памяти)
	// Для полной реализации нужно добавить методы сохранения в БД

	s.writeJSONResponse(w, map[string]interface{}{
		"success":  true,
		"strategy": strategy,
	}, http.StatusOK)
}

// handleGetStrategies получает список стратегий
func (s *Server) handleGetStrategies(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	strategyManager := classification.NewStrategyManager()
	strategies := strategyManager.GetAllStrategies()

	s.writeJSONResponse(w, map[string]interface{}{
		"strategies": strategies,
	}, http.StatusOK)
}

// handleGetAvailableStrategies получает доступные стратегии классификации
func (s *Server) handleGetAvailableStrategies(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	strategyManager := classification.NewStrategyManager()
	strategies := strategyManager.GetAllStrategies()

	// Фильтруем по клиенту если указан (используем ID в метаданных стратегии)
	clientIDStr := r.URL.Query().Get("client_id")
	var filteredStrategies []classification.FoldingStrategyConfig

	for _, strategy := range strategies {
		if clientIDStr != "" {
			// Проверяем, содержится ли client_id в метаданных или имени стратегии
			clientID, err := strconv.Atoi(clientIDStr)
			if err == nil {
				// Проверяем, есть ли client_id в имени стратегии (простая реализация)
				if fmt.Sprintf("client_%d", clientID) == strategy.ID[:len(fmt.Sprintf("client_%d", clientID))] {
					filteredStrategies = append(filteredStrategies, strategy)
				}
			}
		} else {
			// Если клиент не указан, включаем только глобальные стратегии
			if strategy.ID != "" && !strings.HasPrefix(strategy.ID, "client_") {
				filteredStrategies = append(filteredStrategies, strategy)
			}
		}
	}

	s.writeJSONResponse(w, map[string]interface{}{
		"strategies":    filteredStrategies,
		"total_count":   len(filteredStrategies),
		"client_filter": clientIDStr,
	}, http.StatusOK)
}

// handleGetClientStrategies получает стратегии для конкретного клиента
func (s *Server) handleGetClientStrategies(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	clientIDStr := r.URL.Query().Get("client_id")
	if clientIDStr == "" {
		s.writeJSONError(w, "client_id is required", http.StatusBadRequest)
		return
	}

	clientID, err := strconv.Atoi(clientIDStr)
	if err != nil {
		s.writeJSONError(w, "Invalid client_id", http.StatusBadRequest)
		return
	}

	// Получаем стратегии из БД
	strategies, err := s.db.GetFoldingStrategiesByClient(clientID)
	if err != nil {
		s.writeJSONError(w, fmt.Sprintf("Failed to get strategies: %v", err), http.StatusInternalServerError)
		return
	}

	// Преобразуем в формат ответа
	var responseStrategies []map[string]interface{}
	for _, strategy := range strategies {
		responseStrategies = append(responseStrategies, map[string]interface{}{
			"id":              strategy.ID,
			"name":            strategy.Name,
			"description":     strategy.Description,
			"strategy_config": strategy.StrategyConfig,
			"client_id":       strategy.ClientID,
			"is_default":      strategy.IsDefault,
			"created_at":      strategy.CreatedAt,
			"updated_at":      strategy.UpdatedAt,
		})
	}

	s.writeJSONResponse(w, map[string]interface{}{
		"client_id":   clientID,
		"strategies":  responseStrategies,
		"total_count": len(responseStrategies),
	}, http.StatusOK)
}

// handleCreateOrUpdateClientStrategy создает или обновляет стратегию клиента
func (s *Server) handleCreateOrUpdateClientStrategy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req StrategyConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.ClientID == 0 {
		s.writeJSONError(w, "client_id is required", http.StatusBadRequest)
		return
	}

	if req.MaxDepth <= 0 {
		req.MaxDepth = 2
	}

	// Создаем конфигурацию стратегии
	strategyConfig := classification.FoldingStrategyConfig{
		Name:        req.Name,
		Description: req.Description,
		MaxDepth:    req.MaxDepth,
		Priority:    req.Priority,
		Rules:       req.Rules,
	}

	// Сериализуем конфигурацию в JSON
	configJSON, err := json.Marshal(strategyConfig)
	if err != nil {
		s.writeJSONError(w, fmt.Sprintf("Failed to serialize strategy config: %v", err), http.StatusInternalServerError)
		return
	}

	// Создаем стратегию для БД
	dbStrategy := &database.FoldingStrategy{
		Name:           req.Name,
		Description:    req.Description,
		StrategyConfig: string(configJSON),
		ClientID:       &req.ClientID,
		IsDefault:      false, // По умолчанию не делаем стратегию дефолтной
	}

	// Сохраняем в БД
	createdStrategy, err := s.db.CreateFoldingStrategy(dbStrategy)
	if err != nil {
		s.writeJSONError(w, fmt.Sprintf("Failed to create strategy: %v", err), http.StatusInternalServerError)
		return
	}

	s.writeJSONResponse(w, map[string]interface{}{
		"success": true,
		"strategy": map[string]interface{}{
			"id":              createdStrategy.ID,
			"name":            createdStrategy.Name,
			"description":     createdStrategy.Description,
			"strategy_config": createdStrategy.StrategyConfig,
			"client_id":       createdStrategy.ClientID,
			"is_default":      createdStrategy.IsDefault,
			"created_at":      createdStrategy.CreatedAt,
			"updated_at":      createdStrategy.UpdatedAt,
		},
		"message":     "Strategy created successfully",
		"strategy_id": createdStrategy.ID,
	}, http.StatusCreated)
}

// handleClassifyItemDirect выполняет прямую классификацию элемента
func (s *Server) handleClassifyItemDirect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ItemName   string                 `json:"item_name"`
		ItemCode   string                 `json:"item_code"`
		StrategyID string                 `json:"strategy_id"`
		Context    map[string]interface{} `json:"context,omitempty"`
		Category   string                 `json:"category,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.ItemName == "" {
		s.writeJSONError(w, "item_name is required", http.StatusBadRequest)
		return
	}

	if req.StrategyID == "" {
		req.StrategyID = "top_priority"
	}

	// Проверяем API ключ
	apiKey := os.Getenv("ARLIAI_API_KEY")
	if apiKey == "" {
		s.writeJSONError(w, "ARLIAI_API_KEY not set", http.StatusBadRequest)
		return
	}

	// Получаем модель из WorkerConfigManager
	model := s.getModelFromConfig()
	aiClassifier := classification.NewAIClassifier(apiKey, model)

	// Определяем категорию
	category := req.Category
	if category == "" {
		category = "общее" // Дефолтная категория
	}

	// Выполняем AI классификацию
	aiRequest := classification.AIClassificationRequest{
		ItemName:    req.ItemName,
		Description: req.ItemCode,
		Context:     req.Context,
		MaxLevels:   5,
	}

	aiResponse, err := aiClassifier.ClassifyWithAI(aiRequest)
	if err != nil {
		s.writeJSONError(w, fmt.Sprintf("Classification failed: %v", err), http.StatusInternalServerError)
		return
	}

	s.writeJSONResponse(w, map[string]interface{}{
		"item_name":     req.ItemName,
		"item_code":     req.ItemCode,
		"original_name": req.ItemName,
		"category":      aiResponse.CategoryPath,
		"confidence":    aiResponse.Confidence,
		"reasoning":     aiResponse.Reasoning,
		"strategy":      req.StrategyID,
		"classification": map[string]interface{}{
			"category_path": aiResponse.CategoryPath,
			"confidence":    aiResponse.Confidence,
			"reasoning":     aiResponse.Reasoning,
			"alternatives":  aiResponse.Alternatives,
		},
	}, http.StatusOK)
}
