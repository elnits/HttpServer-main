package server

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"httpserver/database"
	"httpserver/nomenclature"
)

// ProviderConfig конфигурация провайдера AI
type ProviderConfig struct {
	Name         string            `json:"name"`          // Название провайдера (arliai, openai, etc.)
	APIKey       string            `json:"api_key"`       // API ключ (скрыт в ответах)
	BaseURL      string            `json:"base_url"`      // Базовый URL API
	Enabled      bool              `json:"enabled"`       // Включен ли провайдер
	Priority     int               `json:"priority"`      // Приоритет (меньше = выше приоритет)
	MaxWorkers   int               `json:"max_workers"`   // Максимальное количество воркеров
	RateLimit    int               `json:"rate_limit"`    // Лимит запросов в минуту
	Timeout      time.Duration     `json:"timeout"`       // Таймаут запросов
	Models       []ModelConfig     `json:"models"`        // Доступные модели
	Metadata     map[string]string `json:"metadata"`      // Дополнительные метаданные
}

// ModelConfig конфигурация модели AI
type ModelConfig struct {
	Name         string  `json:"name"`          // Название модели
	Provider     string  `json:"provider"`      // Провайдер модели
	Enabled      bool    `json:"enabled"`       // Включена ли модель
	Priority     int     `json:"priority"`      // Приоритет модели (меньше = выше)
	MaxTokens    int     `json:"max_tokens"`     // Максимальное количество токенов
	Temperature  float64 `json:"temperature"`   // Температура для генерации
	CostPerToken float64 `json:"cost_per_token"` // Стоимость за токен
	Speed        string  `json:"speed"`         // Скорость: fast, medium, slow
	Quality      string  `json:"quality"`       // Качество: high, medium, low
}

// WorkerConfigManager управляет конфигурацией воркеров и моделей
type WorkerConfigManager struct {
	mu                sync.RWMutex
	providers         map[string]*ProviderConfig
	defaultProvider   string
	defaultModel      string
	globalMaxWorkers  int
	configFilePath    string
	serviceDB         *database.ServiceDB // Добавить это поле
}

// NewWorkerConfigManager создает новый менеджер конфигурации
func NewWorkerConfigManager(serviceDB *database.ServiceDB) *WorkerConfigManager {
	manager := &WorkerConfigManager{
		providers:        make(map[string]*ProviderConfig),
		defaultProvider:  "arliai",
		defaultModel:     "GLM-4.5-Air",
		globalMaxWorkers: 2,
		configFilePath:   "worker_config.json",
		serviceDB:        serviceDB, // Добавить это
	}

	// Инициализация дефолтной конфигурации
	manager.initDefaultConfig()

	// Загрузка конфигурации из файла (если есть)
	manager.loadConfig()

	return manager
}

// initDefaultConfig инициализирует дефолтную конфигурацию
func (wcm *WorkerConfigManager) initDefaultConfig() {
	wcm.mu.Lock()
	defer wcm.mu.Unlock()

	// Дефолтная конфигурация для Arliai
	// Загружаем API ключ из переменных окружения
	apiKey := os.Getenv("ARLIAI_API_KEY")
	
	arliaiConfig := &ProviderConfig{
		Name:       "arliai",
		APIKey:     apiKey,
		BaseURL:    "https://api.arliai.com/v1/chat/completions",
		Enabled:    true,
		Priority:   1,
		MaxWorkers: 2,
		RateLimit:  120, // 120 запросов в минуту
		Timeout:    60 * time.Second,
		Models: []ModelConfig{
			{
				Name:        "GLM-4.5-Air",
				Provider:    "arliai",
				Enabled:     true,
				Priority:    1,
				MaxTokens:   4096,
				Temperature: 0.3,
				Speed:       "fast",
				Quality:     "high",
			},
			{
				Name:        "GLM-4.5",
				Provider:    "arliai",
				Enabled:     true,
				Priority:    2,
				MaxTokens:   8192,
				Temperature: 0.3,
				Speed:       "medium",
				Quality:     "high",
			},
		},
		Metadata: make(map[string]string),
	}

	wcm.providers["arliai"] = arliaiConfig
}

// GetConfig возвращает текущую конфигурацию (без API ключей)
func (wcm *WorkerConfigManager) GetConfig() map[string]interface{} {
	wcm.mu.RLock()
	defer wcm.mu.RUnlock()

	providers := make(map[string]interface{})
	for name, provider := range wcm.providers {
		// Создаем копию без API ключа
		safeProvider := *provider
		hasAPIKey := safeProvider.APIKey != "" // Запоминаем, есть ли API ключ
		safeProvider.APIKey = "" // Скрываем API ключ
		// Преобразуем в map для JSON сериализации
		providerMap := map[string]interface{}{
			"name":        safeProvider.Name,
			"base_url":    safeProvider.BaseURL,
			"enabled":     safeProvider.Enabled,
			"priority":    safeProvider.Priority,
			"max_workers": safeProvider.MaxWorkers,
			"rate_limit":  safeProvider.RateLimit,
			"timeout":     safeProvider.Timeout.String(),
			"models":      safeProvider.Models,
			"metadata":    safeProvider.Metadata,
			"has_api_key": hasAPIKey, // Добавляем информацию о наличии ключа
		}
		providers[name] = providerMap
	}

	return map[string]interface{}{
		"providers":        providers,
		"default_provider": wcm.defaultProvider,
		"default_model":    wcm.defaultModel,
		"global_max_workers": wcm.globalMaxWorkers,
	}
}

// UpdateProvider обновляет конфигурацию провайдера
func (wcm *WorkerConfigManager) UpdateProvider(name string, config *ProviderConfig) error {
	wcm.mu.Lock()
	defer wcm.mu.Unlock()

	if config == nil {
		return fmt.Errorf("config cannot be nil")
	}

	// Сохраняем старый API ключ, если он не указан в новом конфиге
	if existing, ok := wcm.providers[name]; ok && config.APIKey == "" {
		config.APIKey = existing.APIKey
	}

	config.Name = name
	wcm.providers[name] = config

	// Сохраняем конфигурацию
	return wcm.saveConfig()
}

// UpdateModel обновляет конфигурацию модели
func (wcm *WorkerConfigManager) UpdateModel(providerName, modelName string, modelConfig *ModelConfig) error {
	wcm.mu.Lock()
	defer wcm.mu.Unlock()

	provider, ok := wcm.providers[providerName]
	if !ok {
		return fmt.Errorf("provider %s not found", providerName)
	}

	// Ищем модель и обновляем или добавляем
	found := false
	for i, model := range provider.Models {
		if model.Name == modelName {
			modelConfig.Name = modelName
			modelConfig.Provider = providerName
			provider.Models[i] = *modelConfig
			found = true
			break
		}
	}

	if !found {
		modelConfig.Name = modelName
		modelConfig.Provider = providerName
		provider.Models = append(provider.Models, *modelConfig)
	}

	// Сохраняем конфигурацию
	return wcm.saveConfig()
}

// SetDefaultProvider устанавливает провайдера по умолчанию
func (wcm *WorkerConfigManager) SetDefaultProvider(name string) error {
	wcm.mu.Lock()
	defer wcm.mu.Unlock()

	if _, ok := wcm.providers[name]; !ok {
		return fmt.Errorf("provider %s not found", name)
	}

	wcm.defaultProvider = name
	return wcm.saveConfig()
}

// SetDefaultModel устанавливает модель по умолчанию
func (wcm *WorkerConfigManager) SetDefaultModel(providerName, modelName string) error {
	wcm.mu.Lock()
	defer wcm.mu.Unlock()

	provider, ok := wcm.providers[providerName]
	if !ok {
		return fmt.Errorf("provider %s not found", providerName)
	}

	// Проверяем, что модель существует
	found := false
	for _, model := range provider.Models {
		if model.Name == modelName {
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("model %s not found in provider %s", modelName, providerName)
	}

	wcm.defaultProvider = providerName
	wcm.defaultModel = modelName
	return wcm.saveConfig()
}

// SetGlobalMaxWorkers устанавливает глобальный максимум воркеров
func (wcm *WorkerConfigManager) SetGlobalMaxWorkers(maxWorkers int) error {
	if maxWorkers < 1 || maxWorkers > 100 {
		return fmt.Errorf("max workers must be between 1 and 100")
	}

	wcm.mu.Lock()
	defer wcm.mu.Unlock()

	wcm.globalMaxWorkers = maxWorkers
	return wcm.saveConfig()
}

// GetActiveProvider возвращает активный провайдер (с наивысшим приоритетом)
func (wcm *WorkerConfigManager) GetActiveProvider() (*ProviderConfig, error) {
	wcm.mu.RLock()
	defer wcm.mu.RUnlock()

	var activeProvider *ProviderConfig
	lowestPriority := 999

	for _, provider := range wcm.providers {
		if !provider.Enabled {
			continue
		}
		if provider.Priority < lowestPriority {
			lowestPriority = provider.Priority
			activeProvider = provider
		}
	}

	if activeProvider == nil {
		return nil, fmt.Errorf("no active provider found")
	}

	return activeProvider, nil
}

// GetActiveModel возвращает активную модель для провайдера
func (wcm *WorkerConfigManager) GetActiveModel(providerName string) (*ModelConfig, error) {
	wcm.mu.RLock()
	defer wcm.mu.RUnlock()

	provider, ok := wcm.providers[providerName]
	if !ok {
		return nil, fmt.Errorf("provider %s not found", providerName)
	}

	var activeModel *ModelConfig
	lowestPriority := 999

	for i := range provider.Models {
		model := &provider.Models[i]
		if !model.Enabled {
			continue
		}
		if model.Priority < lowestPriority {
			lowestPriority = model.Priority
			activeModel = model
		}
	}

	if activeModel == nil {
		return nil, fmt.Errorf("no active model found for provider %s", providerName)
	}

	return activeModel, nil
}

// CreateAIClient создает AI клиент на основе текущей конфигурации
func (wcm *WorkerConfigManager) CreateAIClient() (*nomenclature.AIClient, error) {
	provider, err := wcm.GetActiveProvider()
	if err != nil {
		return nil, err
	}

	// Если API ключ не установлен в конфигурации, пытаемся получить из переменной окружения
	apiKey := provider.APIKey
	if apiKey == "" {
		// Пытаемся получить из переменной окружения
		// Это делается вне блокировки, так как os.Getenv потокобезопасен
		apiKey = os.Getenv("ARLIAI_API_KEY")
		if apiKey == "" {
			return nil, fmt.Errorf("API key not set for provider %s and ARLIAI_API_KEY environment variable not set", provider.Name)
		}
	}

	model, err := wcm.GetActiveModel(provider.Name)
	if err != nil {
		// Используем дефолтную модель, если не найдена активная
		wcm.mu.RLock()
		defaultModel := wcm.defaultModel
		wcm.mu.RUnlock()
		model = &ModelConfig{Name: defaultModel}
	}

	return nomenclature.NewAIClient(apiKey, model.Name), nil
}

// GetModelAndAPIKey возвращает активную модель и API ключ для использования вне WorkerConfigManager
func (wcm *WorkerConfigManager) GetModelAndAPIKey() (apiKey string, modelName string, err error) {
	provider, err := wcm.GetActiveProvider()
	if err != nil {
		return "", "", err
	}

	apiKey = provider.APIKey
	if apiKey == "" {
		apiKey = os.Getenv("ARLIAI_API_KEY")
		if apiKey == "" {
			return "", "", fmt.Errorf("API key not set for provider %s and ARLIAI_API_KEY environment variable not set", provider.Name)
		}
	}

	model, err := wcm.GetActiveModel(provider.Name)
	if err != nil {
		// Используем дефолтную модель, если не найдена активная
		wcm.mu.RLock()
		defaultModel := wcm.defaultModel
		wcm.mu.RUnlock()
		modelName = defaultModel
	} else {
		modelName = model.Name
	}

	return apiKey, modelName, nil
}

// loadConfig загружает конфигурацию из сервисной БД
func (wcm *WorkerConfigManager) loadConfig() {
	wcm.mu.Lock()
	defer wcm.mu.Unlock()

	// Если ServiceDB не доступна, используем только дефолтную конфигурацию
	if wcm.serviceDB == nil {
		log.Printf("ServiceDB not available, using default config only")
		return
	}

	// Загружаем конфигурацию из БД
	configJSON, err := wcm.serviceDB.GetWorkerConfig()
	if err != nil {
		log.Printf("Error loading config from database: %v, using default config", err)
		return
	}

	// Если конфигурация не найдена, используем дефолтную
	if configJSON == "" {
		log.Printf("No saved config found, using default config")
		return
	}

	// Парсим JSON
	var configData map[string]interface{}
	if err := json.Unmarshal([]byte(configJSON), &configData); err != nil {
		log.Printf("Error parsing config JSON: %v, using default config", err)
		return
	}

	// Восстанавливаем провайдеров
	if providersData, ok := configData["providers"].(map[string]interface{}); ok {
		loadedCount := 0
		for name, providerData := range providersData {
			providerMap, ok := providerData.(map[string]interface{})
			if !ok {
				log.Printf("Invalid provider data for %s, skipping", name)
				continue
			}

			// Создаем ProviderConfig вручную для правильной обработки типов
			provider := ProviderConfig{
				Name:       getString(providerMap, "name"),
				APIKey:     getString(providerMap, "api_key"),
				BaseURL:    getString(providerMap, "base_url"),
				Enabled:    getBool(providerMap, "enabled"),
				Priority:   getInt(providerMap, "priority"),
				MaxWorkers: getInt(providerMap, "max_workers"),
				RateLimit:  getInt(providerMap, "rate_limit"),
			}

			// Обрабатываем timeout (может быть строкой или числом)
			if timeoutStr, ok := providerMap["timeout"].(string); ok {
				if duration, err := time.ParseDuration(timeoutStr); err == nil {
					provider.Timeout = duration
				} else {
					log.Printf("Error parsing timeout for %s: %v, using default", name, err)
					provider.Timeout = 60 * time.Second
				}
			} else if timeoutNum, ok := providerMap["timeout"].(float64); ok {
				// Если сохранено как число (наносекунды)
				provider.Timeout = time.Duration(timeoutNum)
			} else {
				provider.Timeout = 60 * time.Second
			}

			// Обрабатываем модели
			if modelsData, ok := providerMap["models"].([]interface{}); ok {
				for _, modelData := range modelsData {
					if modelMap, ok := modelData.(map[string]interface{}); ok {
						model := ModelConfig{
							Name:        getString(modelMap, "name"),
							Provider:    getString(modelMap, "provider"),
							Enabled:     getBool(modelMap, "enabled"),
							Priority:    getInt(modelMap, "priority"),
							MaxTokens:   getInt(modelMap, "max_tokens"),
							Temperature: getFloat64(modelMap, "temperature"),
							Speed:       getString(modelMap, "speed"),
							Quality:     getString(modelMap, "quality"),
						}
						if costPerToken, ok := modelMap["cost_per_token"].(float64); ok {
							model.CostPerToken = costPerToken
						}
						provider.Models = append(provider.Models, model)
					}
				}
			}

			// Обрабатываем metadata
			if metadataData, ok := providerMap["metadata"].(map[string]interface{}); ok {
				provider.Metadata = make(map[string]string)
				for k, v := range metadataData {
					if str, ok := v.(string); ok {
						provider.Metadata[k] = str
					}
				}
			} else {
				provider.Metadata = make(map[string]string)
			}

			wcm.providers[name] = &provider
			loadedCount++
		}
		log.Printf("Loaded %d providers from database", loadedCount)
	} else {
		log.Printf("No providers data found in config")
	}

	// Восстанавливаем дефолтные значения
	if defaultProvider, ok := configData["default_provider"].(string); ok {
		wcm.defaultProvider = defaultProvider
	}
	if defaultModel, ok := configData["default_model"].(string); ok {
		wcm.defaultModel = defaultModel
	}
	if globalMaxWorkers, ok := configData["global_max_workers"].(float64); ok {
		wcm.globalMaxWorkers = int(globalMaxWorkers)
	}

	log.Printf("Config loaded from service database")
}

// saveConfig сохраняет конфигурацию в сервисную БД
func (wcm *WorkerConfigManager) saveConfig() error {
	wcm.mu.RLock()
	defer wcm.mu.RUnlock()

	// Если ServiceDB не доступна, только логируем
	if wcm.serviceDB == nil {
		log.Printf("Config updated in memory (ServiceDB not available)")
		return nil
	}

	// Формируем JSON конфигурации с правильной обработкой time.Duration
	providersData := make(map[string]interface{})
	for name, provider := range wcm.providers {
		providerMap := map[string]interface{}{
			"name":        provider.Name,
			"api_key":     provider.APIKey,
			"base_url":    provider.BaseURL,
			"enabled":     provider.Enabled,
			"priority":    provider.Priority,
			"max_workers": provider.MaxWorkers,
			"rate_limit":  provider.RateLimit,
			"timeout":     provider.Timeout.String(), // Преобразуем в строку
			"models":      provider.Models,
			"metadata":    provider.Metadata,
		}
		providersData[name] = providerMap
	}

	configData := map[string]interface{}{
		"providers":          providersData,
		"default_provider":   wcm.defaultProvider,
		"default_model":      wcm.defaultModel,
		"global_max_workers": wcm.globalMaxWorkers,
	}

	configJSON, err := json.Marshal(configData)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	// Сохраняем в БД
	if err := wcm.serviceDB.SaveWorkerConfig(string(configJSON)); err != nil {
		return fmt.Errorf("failed to save config to database: %w", err)
	}

	log.Printf("Config saved to service database")
	return nil
}

// GetNomenclatureConfig создает конфигурацию для NomenclatureProcessor
func (wcm *WorkerConfigManager) GetNomenclatureConfig() (nomenclature.Config, error) {
	provider, err := wcm.GetActiveProvider()
	if err != nil {
		return nomenclature.Config{}, err
	}

	// Если API ключ не установлен в конфигурации, пытаемся получить из переменной окружения
	apiKey := provider.APIKey
	if apiKey == "" {
		// Пытаемся получить из переменной окружения
		// Это делается вне блокировки, так как os.Getenv потокобезопасен
		apiKey = os.Getenv("ARLIAI_API_KEY")
		if apiKey == "" {
			return nomenclature.Config{}, fmt.Errorf("API key not set for provider %s and ARLIAI_API_KEY environment variable not set", provider.Name)
		}
	}

	model, err := wcm.GetActiveModel(provider.Name)
	if err != nil {
		model = &ModelConfig{Name: wcm.defaultModel}
	}

	// Определяем количество воркеров (минимум из глобального и провайдера)
	wcm.mu.RLock()
	maxWorkers := wcm.globalMaxWorkers
	wcm.mu.RUnlock()
	
	if provider.MaxWorkers > 0 && provider.MaxWorkers < maxWorkers {
		maxWorkers = provider.MaxWorkers
	}

	config := nomenclature.DefaultConfig()
	config.ArliaiAPIKey = apiKey
	config.AIModel = model.Name
	config.MaxWorkers = maxWorkers
	config.RequestTimeout = provider.Timeout
	
	// Рассчитываем задержку на основе rate limit
	if provider.RateLimit > 0 {
		config.RateLimitDelay = time.Duration(60000/provider.RateLimit) * time.Millisecond
	}

	return config, nil
}

// Вспомогательные функции для безопасного извлечения значений из map
func getString(m map[string]interface{}, key string) string {
	if v, ok := m[key]; ok {
		if str, ok := v.(string); ok {
			return str
		}
	}
	return ""
}

func getBool(m map[string]interface{}, key string) bool {
	if v, ok := m[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}

func getInt(m map[string]interface{}, key string) int {
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case int:
			return val
		case float64:
			return int(val)
		case int64:
			return int(val)
		}
	}
	return 0
}

func getFloat64(m map[string]interface{}, key string) float64 {
	if v, ok := m[key]; ok {
		if f, ok := v.(float64); ok {
			return f
		}
	}
	return 0.0
}

