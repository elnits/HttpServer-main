package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

// handleGetWorkerConfig возвращает текущую конфигурацию воркеров и моделей
func (s *Server) handleGetWorkerConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.workerConfigManager == nil {
		s.writeJSONError(w, "Worker config manager not initialized", http.StatusInternalServerError)
		return
	}

	config := s.workerConfigManager.GetConfig()
	s.writeJSONResponse(w, config, http.StatusOK)
}

// handleUpdateWorkerConfig обновляет конфигурацию воркеров
func (s *Server) handleUpdateWorkerConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.workerConfigManager == nil {
		s.writeJSONError(w, "Worker config manager not initialized", http.StatusInternalServerError)
		return
	}

	var req struct {
		Action string                 `json:"action"` // update_provider, update_model, set_default_provider, set_default_model, set_max_workers
		Data   map[string]interface{} `json:"data"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var err error
	var response map[string]interface{}

	switch req.Action {
	case "update_provider":
		var providerConfig ProviderConfig
		if err = mapToStruct(req.Data, &providerConfig); err != nil {
			s.writeJSONError(w, fmt.Sprintf("Invalid provider config: %v", err), http.StatusBadRequest)
			return
		}
		providerName := req.Data["name"].(string)
		err = s.workerConfigManager.UpdateProvider(providerName, &providerConfig)
		response = map[string]interface{}{"message": "Provider updated successfully"}

	case "update_model":
		var modelConfig ModelConfig
		if err = mapToStruct(req.Data, &modelConfig); err != nil {
			s.writeJSONError(w, fmt.Sprintf("Invalid model config: %v", err), http.StatusBadRequest)
			return
		}
		providerName := req.Data["provider"].(string)
		modelName := req.Data["name"].(string)
		err = s.workerConfigManager.UpdateModel(providerName, modelName, &modelConfig)
		response = map[string]interface{}{"message": "Model updated successfully"}

	case "set_default_provider":
		providerName := req.Data["provider"].(string)
		err = s.workerConfigManager.SetDefaultProvider(providerName)
		response = map[string]interface{}{"message": "Default provider updated successfully"}

	case "set_default_model":
		providerName := req.Data["provider"].(string)
		modelName := req.Data["model"].(string)
		err = s.workerConfigManager.SetDefaultModel(providerName, modelName)
		response = map[string]interface{}{"message": "Default model updated successfully"}

	case "set_max_workers":
		maxWorkers := int(req.Data["max_workers"].(float64))
		err = s.workerConfigManager.SetGlobalMaxWorkers(maxWorkers)
		response = map[string]interface{}{"message": "Global max workers updated successfully"}
	case "set_global_max_workers":
		maxWorkers := int(req.Data["max_workers"].(float64))
		err = s.workerConfigManager.SetGlobalMaxWorkers(maxWorkers)
		response = map[string]interface{}{"message": "Global max workers updated successfully"}

	default:
		s.writeJSONError(w, "Unknown action", http.StatusBadRequest)
		return
	}

	if err != nil {
		s.writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.writeJSONResponse(w, response, http.StatusOK)
}

// handleGetAvailableProviders возвращает список доступных провайдеров
func (s *Server) handleGetAvailableProviders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.workerConfigManager == nil {
		s.writeJSONError(w, "Worker config manager not initialized", http.StatusInternalServerError)
		return
	}

	config := s.workerConfigManager.GetConfig()
	providers := config["providers"].(map[string]interface{})

	// Формируем список провайдеров с их моделями
	providersList := make([]map[string]interface{}, 0)
	for name, providerData := range providers {
		// Преобразуем interface{} в ProviderConfig
		var provider ProviderConfig
		if providerMap, ok := providerData.(map[string]interface{}); ok {
			if err := mapToStruct(providerMap, &provider); err != nil {
				continue
			}
		} else if p, ok := providerData.(ProviderConfig); ok {
			provider = p
		} else {
			continue
		}
		
		providerMap := map[string]interface{}{
			"name":        name,
			"enabled":     provider.Enabled,
			"priority":    provider.Priority,
			"max_workers": provider.MaxWorkers,
			"rate_limit":  provider.RateLimit,
			"models":      provider.Models,
		}
		providersList = append(providersList, providerMap)
	}

	s.writeJSONResponse(w, map[string]interface{}{
		"providers":        providersList,
		"default_provider": config["default_provider"],
		"default_model":    config["default_model"],
	}, http.StatusOK)
}

// handleCheckArliaiConnection проверяет статус подключения к Arliai API
func (s *Server) handleCheckArliaiConnection(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	traceID := r.Header.Get("X-Request-ID")
	if traceID == "" {
		traceID = GenerateTraceID()
	}

	log.Printf("[%s] GET /api/workers/arliai/status", traceID)

	// Проверяем кеш
	if cached, ok := s.arliaiCache.GetStatus(); ok {
		cacheAge := s.arliaiCache.GetStatusAge()
		log.Printf("[%s] Returning cached status (age: %v)", traceID, cacheAge)
		
		response := APIResponse{
			Success:   true,
			Data:      cached,
			Timestamp: time.Now(),
			Duration:  time.Since(startTime),
			Metadata: map[string]interface{}{
				"cached":     true,
				"cache_age_s": cacheAge.Seconds(),
			},
		}
		
		w.Header().Set("X-Request-ID", traceID)
		w.Header().Set("X-Cache", "HIT")
		s.writeJSONResponse(w, response, http.StatusOK)
		return
	}

	// Проверяем WorkerConfigManager для локальной информации
	var localStatus map[string]interface{}
	if s.workerConfigManager != nil {
		provider, err := s.workerConfigManager.GetActiveProvider()
		if err == nil && provider.Name == "arliai" {
			apiKey := provider.APIKey
			if apiKey == "" {
				apiKey = os.Getenv("ARLIAI_API_KEY")
			}

			model, modelErr := s.workerConfigManager.GetActiveModel(provider.Name)
			modelName := ""
			if modelErr == nil {
				modelName = model.Name
			}

			localStatus = map[string]interface{}{
				"provider":    provider.Name,
				"has_api_key": apiKey != "",
				"model":      modelName,
				"enabled":    provider.Enabled,
			}
		}
	}

	// Пытаемся проверить подключение через Arliai API
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	statusResp, err := s.arliaiClient.CheckConnection(ctx, traceID)
	if err != nil {
		// Если API недоступен, возвращаем локальный статус
		log.Printf("[%s] Arliai API check failed: %v, using local status", traceID, err)
		
		connected := false
		if localStatus != nil {
			if hasKey, ok := localStatus["has_api_key"].(bool); ok {
				if enabled, ok2 := localStatus["enabled"].(bool); ok2 {
					connected = hasKey && enabled
				}
			}
		}

		responseData := map[string]interface{}{
			"connected":     connected,
			"api_available": false,
			"last_check":    time.Now(),
		}
		if localStatus != nil {
			responseData["provider"] = localStatus["provider"]
			responseData["has_api_key"] = localStatus["has_api_key"]
			responseData["model"] = localStatus["model"]
			responseData["enabled"] = localStatus["enabled"]
		}
		
		response := APIResponse{
			Success: true,
			Data:    responseData,
			Timestamp: time.Now(),
			Duration:  time.Since(startTime),
			Metadata: map[string]interface{}{
				"cached":        false,
				"api_error":     err.Error(),
			},
		}

		// Кешируем результат даже при ошибке
		s.arliaiCache.SetStatus(response.Data)

		w.Header().Set("X-Request-ID", traceID)
		w.Header().Set("X-Cache", "MISS")
		s.writeJSONResponse(w, response, http.StatusOK)
		return
	}

	// Успешная проверка через API
	connected := statusResp.Status == "ok" || statusResp.Status == "healthy"
	
	responseData := map[string]interface{}{
		"connected":     connected,
		"status":        statusResp.Status,
		"model":         statusResp.Model,
		"version":       statusResp.Version,
		"api_available": true,
		"last_check":    statusResp.Timestamp,
		"response_time_ms": time.Since(startTime).Milliseconds(),
	}

	// Объединяем с локальной информацией
	if localStatus != nil {
		responseData["provider"] = localStatus["provider"]
		responseData["enabled"] = localStatus["enabled"]
		if responseData["model"] == "" {
			responseData["model"] = localStatus["model"]
		}
	}

	response := APIResponse{
		Success:   true,
		Data:      responseData,
		Timestamp: time.Now(),
		Duration:  time.Since(startTime),
		Metadata: map[string]interface{}{
			"cached": false,
		},
	}

	// Кешируем успешный результат
	s.arliaiCache.SetStatus(responseData)

	log.Printf("[%s] Status check completed (duration: %v, connected: %v)", traceID, time.Since(startTime), connected)

	w.Header().Set("X-Request-ID", traceID)
	w.Header().Set("X-Cache", "MISS")
	s.writeJSONResponse(w, response, http.StatusOK)
}

// handleGetModels возвращает список доступных моделей для активного провайдера
func (s *Server) handleGetModels(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	traceID := r.Header.Get("X-Request-ID")
	if traceID == "" {
		traceID = GenerateTraceID()
	}

	log.Printf("[%s] GET /api/workers/models", traceID)

	// Парсим query параметры для фильтрации
	query := r.URL.Query()
	filterStatus := query.Get("status")      // active, deprecated, beta, all
	filterEnabled := query.Get("enabled")    // true, false, all
	searchQuery := query.Get("search")        // поиск по имени модели

	if r.Method != http.MethodGet {
		errorResponse := APIResponse{
			Success: false,
			Error: &APIError{
				Code:      "METHOD_NOT_ALLOWED",
				Message:   "Method not allowed",
				TraceID:   traceID,
				Timestamp: time.Now(),
			},
			Timestamp: time.Now(),
		}
		w.Header().Set("X-Request-ID", traceID)
		w.WriteHeader(http.StatusMethodNotAllowed)
		s.writeJSONResponse(w, errorResponse, http.StatusMethodNotAllowed)
		return
	}

	// Проверяем кеш
	if cached, ok := s.arliaiCache.GetModels(); ok {
		cacheAge := s.arliaiCache.GetModelsAge()
		log.Printf("[%s] Returning cached models (age: %v)", traceID, cacheAge)
		
		response := APIResponse{
			Success:   true,
			Data:      cached,
			Timestamp: time.Now(),
			Duration:  time.Since(startTime),
			Metadata: map[string]interface{}{
				"cached":      true,
				"cache_age_s": cacheAge.Seconds(),
			},
		}
		
		w.Header().Set("X-Request-ID", traceID)
		w.Header().Set("X-Cache", "HIT")
		s.writeJSONResponse(w, response, http.StatusOK)
		return
	}

	if s.workerConfigManager == nil {
		errorResponse := APIResponse{
			Success: false,
			Error: &APIError{
				Code:      "SERVICE_UNAVAILABLE",
				Message:   "Worker config manager not initialized",
				TraceID:   traceID,
				Timestamp: time.Now(),
			},
			Timestamp: time.Now(),
		}
		w.Header().Set("X-Request-ID", traceID)
		s.writeJSONResponse(w, errorResponse, http.StatusInternalServerError)
		return
	}

	// Получаем активный провайдер
	provider, err := s.workerConfigManager.GetActiveProvider()
	if err != nil {
		errorResponse := APIResponse{
			Success: false,
			Error: &APIError{
				Code:      "NO_ACTIVE_PROVIDER",
				Message:   fmt.Sprintf("No active provider: %v", err),
				TraceID:   traceID,
				Timestamp: time.Now(),
			},
			Timestamp: time.Now(),
		}
		w.Header().Set("X-Request-ID", traceID)
		s.writeJSONResponse(w, errorResponse, http.StatusBadRequest)
		return
	}

	// Пытаемся получить модели из Arliai API
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	apiModels, apiErr := s.arliaiClient.GetModels(ctx, traceID)
	
	// Формируем список моделей из локальной конфигурации
	localModels := make([]map[string]interface{}, 0)
	for _, model := range provider.Models {
		if model.Enabled {
			modelData := map[string]interface{}{
				"id":          model.Name,
				"name":        model.Name,
				"provider":    model.Provider,
				"enabled":     model.Enabled,
				"priority":    model.Priority,
				"max_tokens":  model.MaxTokens,
				"temperature": model.Temperature,
				"speed":       model.Speed,
				"quality":     model.Quality,
				"status":      "active",
			}
			localModels = append(localModels, modelData)
		}
	}

	// Объединяем с моделями из API, если доступны
	var finalModels []map[string]interface{}
	if apiErr == nil && len(apiModels) > 0 {
		// Используем модели из API как основной источник
		modelMap := make(map[string]map[string]interface{})
		for _, apiModel := range apiModels {
			modelMap[apiModel.ID] = map[string]interface{}{
				"id":          apiModel.ID,
				"name":        apiModel.Name,
				"provider":    provider.Name,
				"speed":       apiModel.Speed,
				"quality":     apiModel.Quality,
				"description": apiModel.Description,
				"status":      apiModel.Status,
				"max_tokens":  apiModel.MaxTokens,
				"tags":        apiModel.Tags,
			}
		}
		// Объединяем с локальными моделями
		for _, localModel := range localModels {
			modelID := ""
			if id, ok := localModel["id"].(string); ok {
				modelID = id
			} else if name, ok := localModel["name"].(string); ok {
				modelID = name
			}
			
			if modelID != "" {
				if existing, ok := modelMap[modelID]; ok {
					// Обновляем существующую модель локальными данными
					for k, v := range localModel {
						existing[k] = v
					}
				} else {
					modelMap[modelID] = localModel
				}
			}
		}
		// Преобразуем обратно в слайс
		finalModels = make([]map[string]interface{}, 0, len(modelMap))
		for _, model := range modelMap {
			finalModels = append(finalModels, model)
		}

		// Сохраняем полученные модели в конфигурацию провайдера
		updatedModels := make([]ModelConfig, 0, len(finalModels))
		for _, modelData := range finalModels {
			modelConfig := ModelConfig{
				Name:         getString(modelData, "name"),
				Provider:     provider.Name,
				Enabled:      getBool(modelData, "enabled"),
				Priority:     getInt(modelData, "priority"),
				MaxTokens:    getInt(modelData, "max_tokens"),
				Temperature:  getFloat64(modelData, "temperature"),
				Speed:        getString(modelData, "speed"),
				Quality:      getString(modelData, "quality"),
				CostPerToken: getFloat64(modelData, "cost_per_token"),
			}

			// Если модель не имеет некоторых базовых параметров, устанавливаем значения по умолчанию
			if modelConfig.Name == "" {
				modelConfig.Name = getString(modelData, "id")
			}
			if modelConfig.Temperature == 0.0 {
				modelConfig.Temperature = 0.3
			}
			if modelConfig.Speed == "" {
				modelConfig.Speed = "medium"
			}
			if modelConfig.Quality == "" {
				modelConfig.Quality = "high"
			}

			updatedModels = append(updatedModels, modelConfig)
		}

		// Обновляем провайдера с новыми моделями
		updatedProvider := *provider
		updatedProvider.Models = updatedModels

		if err := s.workerConfigManager.UpdateProvider(provider.Name, &updatedProvider); err != nil {
			log.Printf("[%s] Failed to save updated models: %v", traceID, err)
		} else {
			log.Printf("[%s] Successfully saved %d models to configuration", traceID, len(updatedModels))
		}
	} else {
		// Используем только локальные модели
		finalModels = localModels
		log.Printf("[%s] Using local models only (API error: %v)", traceID, apiErr)
	}

	// Применяем фильтры
	filteredModels := make([]map[string]interface{}, 0)
	for _, model := range finalModels {
		// Фильтр по статусу
		if filterStatus != "" && filterStatus != "all" {
			modelStatus := ""
			if status, ok := model["status"].(string); ok {
				modelStatus = status
			}
			if modelStatus != filterStatus {
				continue
			}
		}

		// Фильтр по enabled
		if filterEnabled != "" && filterEnabled != "all" {
			modelEnabled := model["enabled"]
			if filterEnabled == "true" && modelEnabled != true {
				continue
			}
			if filterEnabled == "false" && modelEnabled != false {
				continue
			}
		}

		// Поиск по имени
		if searchQuery != "" {
			modelName := ""
			if name, ok := model["name"].(string); ok {
				modelName = name
			} else if id, ok := model["id"].(string); ok {
				modelName = id
			}
			if !strings.Contains(strings.ToLower(modelName), strings.ToLower(searchQuery)) {
				continue
			}
		}

		filteredModels = append(filteredModels, model)
	}

	// Сортируем по приоритету, затем по имени
	sort.Slice(filteredModels, func(i, j int) bool {
		priI, okI := filteredModels[i]["priority"].(int)
		priJ, okJ := filteredModels[j]["priority"].(int)
		
		// Если приоритеты равны или отсутствуют, сортируем по имени
		if !okI || !okJ || priI == priJ {
			nameI := ""
			if n, ok := filteredModels[i]["name"].(string); ok {
				nameI = n
			} else if id, ok := filteredModels[i]["id"].(string); ok {
				nameI = id
			}
			nameJ := ""
			if n, ok := filteredModels[j]["name"].(string); ok {
				nameJ = n
			} else if id, ok := filteredModels[j]["id"].(string); ok {
				nameJ = id
			}
			return nameI < nameJ
		}
		
		return priI < priJ
	})

	config := s.workerConfigManager.GetConfig()
	defaultModel := ""
	if dm, ok := config["default_model"].(string); ok {
		defaultModel = dm
	}
	
	// Помечаем модель по умолчанию
	for i := range filteredModels {
		modelName := ""
		if name, ok := filteredModels[i]["name"].(string); ok {
			modelName = name
		} else if id, ok := filteredModels[i]["id"].(string); ok {
			modelName = id
		}
		
		if modelName == defaultModel || filteredModels[i]["is_default"] == true {
			filteredModels[i]["is_default"] = true
		} else {
			filteredModels[i]["is_default"] = false
		}
	}

	responseData := map[string]interface{}{
		"models":        filteredModels,
		"provider":      provider.Name,
		"default_model": defaultModel,
		"total":         len(filteredModels),
		"total_before_filter": len(finalModels),
		"api_available": apiErr == nil,
		"filters": map[string]interface{}{
			"status":  filterStatus,
			"enabled": filterEnabled,
			"search":  searchQuery,
		},
	}

	if apiErr != nil {
		responseData["api_error"] = apiErr.Error()
	}

	response := APIResponse{
		Success:   true,
		Data:      responseData,
		Timestamp: time.Now(),
		Duration:  time.Since(startTime),
		Metadata: map[string]interface{}{
			"cached": false,
		},
	}

	// Кешируем результат
	s.arliaiCache.SetModels(responseData)

	log.Printf("[%s] Models fetch completed (duration: %v, count: %d)", traceID, time.Since(startTime), len(finalModels))

	w.Header().Set("X-Request-ID", traceID)
	w.Header().Set("X-Cache", "MISS")
	s.writeJSONResponse(w, response, http.StatusOK)
}

// mapToStruct преобразует map в структуру
func mapToStruct(m map[string]interface{}, target interface{}) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

