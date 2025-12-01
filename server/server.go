package server

import (
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"httpserver/database"
	"httpserver/nomenclature"
	"httpserver/normalization"
	"httpserver/quality"
	"httpserver/server/middleware"

	"github.com/google/uuid"
)

// Server HTTP сервер для приема данных из 1С
type Server struct {
	db                      *database.DB
	normalizedDB            *database.DB
	serviceDB               *database.ServiceDB
	unifiedCatalogsDB       *database.DB // Единая БД справочников
	currentDBPath           string
	currentNormalizedDBPath string
	config                  *Config
	httpServer              *http.Server
	logChan                 chan LogEntry
	nomenclatureProcessor   *nomenclature.NomenclatureProcessor
	processorMutex          sync.RWMutex
	normalizer              *normalization.Normalizer
	normalizerEvents        chan string
	normalizerRunning       bool
	normalizerMutex         sync.RWMutex
	normalizerStartTime     time.Time
	normalizerProcessed     int
	normalizerSuccess       int
	normalizerErrors        int
	dbMutex                 sync.RWMutex
	shutdownChan            chan struct{}
	startTime               time.Time
	qualityAnalyzer         *quality.QualityAnalyzer
	workerConfigManager     *WorkerConfigManager
	arliaiClient            *ArliaiClient
	arliaiCache             *ArliaiCache
	// Статус анализа качества
	qualityAnalysisRunning bool
	qualityAnalysisMutex   sync.RWMutex
	qualityAnalysisStatus  QualityAnalysisStatus
	// KPVED классификация
	hierarchicalClassifier *normalization.HierarchicalClassifier
	kpvedClassifierMutex   sync.RWMutex
	// Отслеживание текущих задач КПВЭД классификации
	kpvedCurrentTasks      map[int]*classificationTask // workerID -> текущая задача
	kpvedCurrentTasksMutex sync.RWMutex
	// Флаг остановки воркеров КПВЭД классификации
	kpvedWorkersStopped   bool
	kpvedWorkersStopMutex sync.RWMutex
	// Кэш БД для выгрузок (ключ - upload_uuid)
	uploadDBs      map[string]*database.DB
	uploadDBsMutex sync.RWMutex
	// Обратная выгрузка
	exportJobs      map[string]*ExportJob
	exportJobsMutex sync.RWMutex
}

// QualityAnalysisStatus статус анализа качества
type QualityAnalysisStatus struct {
	IsRunning        bool    `json:"is_running"`
	Progress         float64 `json:"progress"`
	Processed        int     `json:"processed"`
	Total            int     `json:"total"`
	CurrentStep      string  `json:"current_step"`
	DuplicatesFound  int     `json:"duplicates_found"`
	ViolationsFound  int     `json:"violations_found"`
	SuggestionsFound int     `json:"suggestions_found"`
	Error            string  `json:"error,omitempty"`
}

// NewServer создает новый сервер (устаревший метод, используйте NewServerWithConfig)
func NewServer(db *database.DB, normalizedDB *database.DB, serviceDB *database.ServiceDB, dbPath, normalizedDBPath, port string) *Server {
	config := &Config{
		Port:                       port,
		DatabasePath:               dbPath,
		NormalizedDatabasePath:     normalizedDBPath,
		ServiceDatabasePath:        "service.db",
		UnifiedCatalogsDBPath:      "unified_catalogs.db",
		LogBufferSize:              100,
		NormalizerEventsBufferSize: 100,
	}
	// unifiedCatalogsDB будет nil, так как старый метод не знает о нём
	return NewServerWithConfig(db, normalizedDB, serviceDB, nil, dbPath, normalizedDBPath, config)
}

// NewServerWithConfig создает новый сервер с конфигурацией
func NewServerWithConfig(db *database.DB, normalizedDB *database.DB, serviceDB *database.ServiceDB, unifiedCatalogsDB *database.DB, dbPath, normalizedDBPath string, config *Config) *Server {
	// Создаем канал событий для нормализатора
	normalizerEvents := make(chan string, config.NormalizerEventsBufferSize)

	// Инициализируем AI конфигурацию для нормализатора
	aiConfig := &normalization.AIConfig{
		Enabled:        true, // Всегда включаем, проверка API ключа будет внутри
		MinConfidence:  0.7,
		RateLimitDelay: 100 * time.Millisecond,
		MaxRetries:     3,
	}

	// Создаем менеджер конфигурации воркеров ПЕРЕД нормализатором
	// чтобы передать его в normalizer для получения API ключа из БД
	workerConfigManager := NewWorkerConfigManager(serviceDB)

	// Создаем нормализатор
	normalizer := normalization.NewNormalizer(db, normalizerEvents, aiConfig)

	// Создаем анализатор качества
	qualityAnalyzer := quality.NewQualityAnalyzer(db)

	// Создаем клиент Arliai и кеш
	arliaiClient := NewArliaiClient()
	arliaiCache := NewArliaiCache()

	// Инициализируем KPVED hierarchical classifier
	var hierarchicalClassifier *normalization.HierarchicalClassifier

	// Пытаемся загрузить KPVED классификатор
	apiKey := os.Getenv("ARLIAI_API_KEY")
	model := os.Getenv("ARLIAI_MODEL")
	if model == "" {
		model = "GLM-4.5-Air" // По умолчанию
	}

	if apiKey != "" && serviceDB != nil {
		aiClient := nomenclature.NewAIClient(apiKey, model)
		var err error
		hierarchicalClassifier, err = normalization.NewHierarchicalClassifier(serviceDB, aiClient)
		if err != nil {
			log.Printf("Warning: Failed to initialize KPVED classifier: %v", err)
			log.Printf("KPVED classification will be disabled. Load classifier via /api/kpved/load-from-file endpoint.")
			hierarchicalClassifier = nil
		} else {
			log.Printf("KPVED hierarchical classifier initialized successfully")
		}
	} else {
		if apiKey == "" {
			log.Printf("Warning: ARLIAI_API_KEY not set. KPVED classification will be disabled.")
		}
		if serviceDB == nil {
			log.Printf("Warning: ServiceDB not initialized. KPVED classification will be disabled.")
		}
	}

	return &Server{
		db:                      db,
		normalizedDB:            normalizedDB,
		serviceDB:               serviceDB,
		unifiedCatalogsDB:       unifiedCatalogsDB, // Единая БД справочников
		currentDBPath:           dbPath,
		currentNormalizedDBPath: normalizedDBPath,
		config:                  config,
		httpServer:              nil,
		logChan:                 make(chan LogEntry, config.LogBufferSize),
		nomenclatureProcessor:   nil,
		normalizer:              normalizer,
		normalizerEvents:        normalizerEvents,
		normalizerRunning:       false,
		shutdownChan:            make(chan struct{}),
		startTime:               time.Now(),
		qualityAnalyzer:         qualityAnalyzer,
		workerConfigManager:     workerConfigManager,
		arliaiClient:            arliaiClient,
		arliaiCache:             arliaiCache,
		hierarchicalClassifier:  hierarchicalClassifier,
		kpvedCurrentTasks:       make(map[int]*classificationTask),
		kpvedWorkersStopped:     false,
		uploadDBs:               make(map[string]*database.DB),
		exportJobs:              make(map[string]*ExportJob),
	}
}

// Start запускает HTTP сервер
func (s *Server) Start() error {
	s.log(LogEntry{
		Timestamp: time.Now(),
		Level:     "INFO",
		Message:   fmt.Sprintf("Starting server on port %s", s.config.Port),
	})

	// Получаем настроенный handler
	handler := s.setupMux()

	// Создаем HTTP сервер с увеличенными таймаутами для длительных операций
	// ReadTimeout и WriteTimeout установлены для защиты от зависших соединений
	// Но для операций классификации КПВЭД нужны большие значения
	s.httpServer = &http.Server{
		Addr:         ":" + s.config.Port,
		Handler:      handler,
		ReadTimeout:  30 * time.Minute,  // Увеличен для длительных операций классификации
		WriteTimeout: 30 * time.Minute,  // Увеличен для длительных операций классификации
		IdleTimeout:  120 * time.Second, // Таймаут для idle соединений
	}

	// Запускаем сервер
	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}

	return nil
}

// setupMux настраивает маршруты и возвращает http.Handler
// Используется как в Start(), так и в ServeHTTP() для тестов
func (s *Server) setupMux() http.Handler {
	mux := http.NewServeMux()

	// Регистрируем обработчики для 1С (старые эндпоинты для обратной совместимости)
	mux.HandleFunc("/handshake", s.handleHandshake)
	mux.HandleFunc("/metadata", s.handleMetadata)
	mux.HandleFunc("/constant", s.handleConstant)
	mux.HandleFunc("/catalog/meta", s.handleCatalogMeta)
	mux.HandleFunc("/catalog/item", s.handleCatalogItem)
	mux.HandleFunc("/catalog/items", s.handleCatalogItems)
	mux.HandleFunc("/complete", s.handleComplete)
	mux.HandleFunc("/stats", s.handleStats)
	mux.HandleFunc("/health", s.handleHealth)

	// Регистрируем новые API v1 эндпоинты
	mux.HandleFunc("/api/v1/upload/handshake", s.handleHandshake)
	mux.HandleFunc("/api/v1/upload/metadata", s.handleMetadata)
	mux.HandleFunc("/api/v1/upload/nomenclature/batch", s.handleNomenclatureBatch)
	mux.HandleFunc("/api/v1/health", s.handleHealth)

	// Регистрируем эндпоинты качества данных (до общих маршрутов для приоритета)
	mux.HandleFunc("/api/v1/upload/", s.handleQualityUploadRoutes)
	// handleQualityDatabaseRoutes обрабатывает маршруты качества и передает остальные в handleDatabaseV1Routes
	mux.HandleFunc("/api/v1/databases/", s.handleQualityDatabaseRoutes)

	// Регистрируем API эндпоинты
	// Важно: регистрируем до статического контента, чтобы не перехватывались запросы
	mux.HandleFunc("/api/uploads", s.handleListUploads)
	mux.HandleFunc("/api/uploads/", s.handleUploadRoutes)
	mux.HandleFunc("/api/exports", s.handleExportsRoot)
	mux.HandleFunc("/api/exports/", s.handleExportRoutes)

	// Регистрируем API эндпоинты для нормализованной БД
	mux.HandleFunc("/api/normalized/uploads", s.handleNormalizedListUploads)
	mux.HandleFunc("/api/normalized/uploads/", s.handleNormalizedUploadRoutes)

	// Регистрируем эндпоинты для приема нормализованных данных
	mux.HandleFunc("/api/normalized/upload/handshake", s.handleNormalizedHandshake)
	mux.HandleFunc("/api/normalized/upload/metadata", s.handleNormalizedMetadata)
	mux.HandleFunc("/api/normalized/upload/constant", s.handleNormalizedConstant)
	mux.HandleFunc("/api/normalized/upload/catalog/meta", s.handleNormalizedCatalogMeta)
	mux.HandleFunc("/api/normalized/upload/catalog/item", s.handleNormalizedCatalogItem)
	mux.HandleFunc("/api/normalized/upload/complete", s.handleNormalizedComplete)

	// Регистрируем API эндпоинты для загрузки данных в 1С
	mux.HandleFunc("/api/1c/databases", s.handle1CDatabasesList)
	mux.HandleFunc("/api/1c/import/handshake", s.handle1CImportHandshake)
	mux.HandleFunc("/api/1c/import/get-constants", s.handle1CImportGetConstants)
	mux.HandleFunc("/api/1c/import/get-catalog", s.handle1CImportGetCatalog)
	mux.HandleFunc("/api/1c/import/complete", s.handle1CImportComplete)

	// Регистрируем эндпоинты для обработки номенклатуры
	mux.HandleFunc("/api/nomenclature/process", s.startNomenclatureProcessing)
	mux.HandleFunc("/api/nomenclature/status", s.getNomenclatureStatus)
	mux.HandleFunc("/api/nomenclature/recent", s.getNomenclatureRecentRecords)
	mux.HandleFunc("/api/nomenclature/pending", s.getNomenclaturePendingRecords)
	mux.HandleFunc("/nomenclature/status", s.serveNomenclatureStatusPage)

	// Регистрируем эндпоинты для нормализации данных
	mux.HandleFunc("/api/normalize/start", s.handleNormalizeStart)
	mux.HandleFunc("/api/normalize/events", s.handleNormalizationEvents)
	mux.HandleFunc("/api/normalization/status", s.handleNormalizationStatus)
	mux.HandleFunc("/api/normalization/stop", s.handleNormalizationStop)
	mux.HandleFunc("/api/normalization/stats", s.handleNormalizationStats)
	mux.HandleFunc("/api/normalization/groups", s.handleNormalizationGroups)
	mux.HandleFunc("/api/normalization/group-items", s.handleNormalizationGroupItems)
	mux.HandleFunc("/api/normalization/item-attributes/", s.handleNormalizationItemAttributes)
	mux.HandleFunc("/api/normalization/export-group", s.handleNormalizationExportGroup)

	// Регистрируем эндпоинты для конфигурации нормализации
	mux.HandleFunc("/api/normalization/config", s.handleNormalizationConfig)

	// Регистрируем эндпоинты для работы со срезами данных
	mux.HandleFunc("/api/snapshots", s.handleSnapshotsRoutes)
	mux.HandleFunc("/api/snapshots/", s.handleSnapshotRoutes)
	mux.HandleFunc("/api/snapshots/auto", s.handleCreateAutoSnapshot)
	mux.HandleFunc("/api/projects/", s.handleProjectSnapshotsRoutes)
	mux.HandleFunc("/api/normalization/databases", s.handleNormalizationDatabases)
	mux.HandleFunc("/api/normalization/tables", s.handleNormalizationTables)
	mux.HandleFunc("/api/normalization/columns", s.handleNormalizationColumns)

	// Регистрируем эндпоинты для КПВЭД классификатора
	mux.HandleFunc("/api/kpved/hierarchy", s.handleKpvedHierarchy)
	mux.HandleFunc("/api/kpved/search", s.handleKpvedSearch)
	mux.HandleFunc("/api/kpved/stats", s.handleKpvedStats)
	mux.HandleFunc("/api/kpved/load", s.handleKpvedLoad)
	// mux.HandleFunc("/api/kpved/load-from-file", s.handleKpvedLoadFromFile) // Метод не реализован
	mux.HandleFunc("/api/kpved/classify-test", s.handleKpvedClassifyTest)
	mux.HandleFunc("/api/kpved/classify-hierarchical", s.handleKpvedClassifyHierarchical)
	mux.HandleFunc("/api/kpved/reclassify", s.handleKpvedReclassify)
	mux.HandleFunc("/api/kpved/reclassify-hierarchical", s.handleKpvedReclassifyHierarchical)
	mux.HandleFunc("/api/kpved/current-tasks", s.handleKpvedCurrentTasks)

	// Регистрируем эндпоинты для управления классификацией
	mux.HandleFunc("/api/kpved/reset", s.handleResetClassification)
	mux.HandleFunc("/api/kpved/reset-all", s.handleResetAllClassification)
	mux.HandleFunc("/api/kpved/reset-by-code", s.handleResetByCode)
	mux.HandleFunc("/api/kpved/reset-low-confidence", s.handleResetLowConfidence)
	mux.HandleFunc("/api/kpved/mark-incorrect", s.handleMarkIncorrect)
	mux.HandleFunc("/api/kpved/mark-correct", s.handleMarkCorrect)
	mux.HandleFunc("/api/kpved/workers/status", s.handleKpvedWorkersStatus)
	mux.HandleFunc("/api/kpved/workers/stop", s.handleKpvedWorkersStop)
	mux.HandleFunc("/api/kpved/workers/resume", s.handleKpvedWorkersResume)
	mux.HandleFunc("/api/kpved/workers/start", s.handleKpvedWorkersResume)
	mux.HandleFunc("/api/kpved/stats/classification", s.handleKpvedStatsGeneral)
	mux.HandleFunc("/api/kpved/stats/by-category", s.handleKpvedStatsByCategory)
	mux.HandleFunc("/api/kpved/stats/incorrect", s.handleKpvedStatsIncorrect)

	// Регистрируем эндпоинты для качества нормализации
	mux.HandleFunc("/api/quality/stats", s.handleQualityStats)
	mux.HandleFunc("/api/quality/item/", s.handleQualityItemDetail)
	mux.HandleFunc("/api/quality/violations", s.handleQualityViolations)
	mux.HandleFunc("/api/quality/violations/", s.handleQualityViolationDetail)
	mux.HandleFunc("/api/quality/suggestions", s.handleQualitySuggestions)
	mux.HandleFunc("/api/quality/suggestions/", s.handleQualitySuggestionAction)
	mux.HandleFunc("/api/quality/duplicates", s.handleQualityDuplicates)
	mux.HandleFunc("/api/quality/duplicates/", s.handleQualityDuplicateAction)
	mux.HandleFunc("/api/quality/assess", s.handleQualityAssess)
	mux.HandleFunc("/api/quality/analyze", s.handleQualityAnalyze)
	mux.HandleFunc("/api/quality/analyze/status", s.handleQualityAnalyzeStatus)

	// Регистрируем эндпоинты для тестирования паттернов
	mux.HandleFunc("/api/patterns/detect", s.handlePatternDetect)
	mux.HandleFunc("/api/patterns/suggest", s.handlePatternSuggest)
	mux.HandleFunc("/api/patterns/test-batch", s.handlePatternTestBatch)

	// Регистрируем эндпоинты для версионирования нормализации
	mux.HandleFunc("/api/normalization/start", s.handleStartNormalization)
	mux.HandleFunc("/api/normalization/apply-patterns", s.handleApplyPatterns)
	mux.HandleFunc("/api/normalization/apply-ai", s.handleApplyAI)
	mux.HandleFunc("/api/normalization/history", s.handleGetSessionHistory)
	mux.HandleFunc("/api/normalization/revert", s.handleRevertStage)
	mux.HandleFunc("/api/normalization/apply-categorization", s.handleApplyCategorization)

	// Регистрируем эндпоинты для классификации
	mux.HandleFunc("/api/classification/classify", s.handleClassifyItem)
	mux.HandleFunc("/api/classification/classify-item", s.handleClassifyItemDirect)
	mux.HandleFunc("/api/classification/strategies", s.handleGetStrategies)
	mux.HandleFunc("/api/classification/strategies/configure", s.handleConfigureStrategy)
	mux.HandleFunc("/api/classification/strategies/client", s.handleGetClientStrategies)
	mux.HandleFunc("/api/classification/strategies/create", s.handleCreateOrUpdateClientStrategy)
	mux.HandleFunc("/api/classification/available", s.handleGetAvailableStrategies)
	mux.HandleFunc("/api/classification/classifiers", s.handleGetClassifiers)

	// Регистрируем эндпоинты для переклассификации
	mux.HandleFunc("/api/reclassification/start", s.handleReclassificationStart)
	mux.HandleFunc("/api/reclassification/events", s.handleReclassificationEvents)
	mux.HandleFunc("/api/reclassification/status", s.handleReclassificationStatus)
	mux.HandleFunc("/api/reclassification/stop", s.handleReclassificationStop)

	// Регистрируем эндпоинты для мониторинга производительности
	mux.HandleFunc("/api/monitoring/metrics", s.handleMonitoringMetrics)
	mux.HandleFunc("/api/monitoring/cache", s.handleMonitoringCache)
	mux.HandleFunc("/api/monitoring/ai", s.handleMonitoringAI)
	mux.HandleFunc("/api/monitoring/history", s.handleMonitoringHistory)
	mux.HandleFunc("/api/monitoring/events", s.handleMonitoringEvents)

	// Регистрируем эндпоинты для управления воркерами и моделями
	mux.HandleFunc("/api/workers/config", s.handleGetWorkerConfig)
	mux.HandleFunc("/api/workers/config/update", s.handleUpdateWorkerConfig)
	mux.HandleFunc("/api/workers/providers", s.handleGetAvailableProviders)
	mux.HandleFunc("/api/workers/arliai/status", s.handleCheckArliaiConnection)
	mux.HandleFunc("/api/workers/models", s.handleGetModels)

	// Регистрируем эндпоинты для работы с базами данных
	mux.HandleFunc("/api/database/info", s.handleDatabaseInfo)
	mux.HandleFunc("/api/databases/list", s.handleDatabasesList)
	mux.HandleFunc("/api/databases/find", s.handleFindDatabase)
	mux.HandleFunc("/api/database/switch", s.handleDatabaseSwitch)
	mux.HandleFunc("/api/databases/analytics", s.handleDatabaseAnalytics)
	mux.HandleFunc("/api/databases/analytics/", s.handleDatabaseAnalytics)
	mux.HandleFunc("/api/databases/history/", s.handleDatabaseHistory)

	// Регистрируем эндпоинты для работы с клиентами
	mux.HandleFunc("/api/clients", s.handleClients)
	mux.HandleFunc("/api/clients/", s.handleClientRoutes)

	// Регистрируем эндпоинт для генерации XML обработки 1С
	mux.HandleFunc("/api/1c/processing/xml", s.handle1CProcessingXML)

	// Статический контент для GUI (регистрируем последним)
	// Используем префикс, чтобы не перехватывать API запросы
	staticFS := http.FileServer(http.Dir("./static/"))
	mux.Handle("/static/", http.StripPrefix("/static/", staticFS))
	// Для корневого пути тоже обрабатываем статику, но только если это не API запрос
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Если это API запрос, возвращаем 404
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		// Иначе отдаем статический контент
		staticFS.ServeHTTP(w, r)
	})

	// Применяем middleware в правильном порядке после регистрации всех маршрутов
	// Порядок важен: сначала SecurityHeaders, затем RequestID, затем Logging, затем существующие middleware
	handler := SecurityHeadersMiddleware(mux)
	handler = RequestIDMiddleware(handler)
	handler = LoggingMiddleware(handler)
	handler = middleware.CORS(handler)
	handler = middleware.RecoverMiddleware(handler)

	return handler
}

// ServeHTTP реализует интерфейс http.Handler для использования в тестах
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	handler := s.setupMux()
	handler.ServeHTTP(w, r)
}

// Shutdown корректно останавливает сервер
func (s *Server) Shutdown(ctx context.Context) error {
	s.log(LogEntry{
		Timestamp: time.Now(),
		Level:     "INFO",
		Message:   "Shutting down server...",
	})

	if s.httpServer != nil {
		return s.httpServer.Shutdown(ctx)
	}
	return nil
}

// log отправляет запись в лог
func (s *Server) log(entry LogEntry) {
	select {
	case s.logChan <- entry:
	default:
		// Если канал полон, пропускаем запись
	}
	log.Printf("[%s] %s: %s", entry.Level, entry.Timestamp.Format("15:04:05"), entry.Message)
}

// writeXMLResponse записывает XML ответ
func (s *Server) writeXMLResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	xmlData, err := xml.MarshalIndent(data, "", "  ")
	if err != nil {
		s.writeErrorResponse(w, "Failed to marshal XML", err)
		return
	}

	w.Write([]byte(xml.Header))
	w.Write(xmlData)
}

// writeErrorResponse записывает ошибку в XML формате
func (s *Server) writeErrorResponse(w http.ResponseWriter, message string, err error) {
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusInternalServerError)

	response := ErrorResponse{
		Success:   false,
		Error:     err.Error(),
		Message:   message,
		Timestamp: time.Now().Format(time.RFC3339),
	}

	xmlData, _ := xml.MarshalIndent(response, "", "  ")
	w.Write([]byte(xml.Header))
	w.Write(xmlData)
}

// generateDatabaseFileName формирует имя файла БД для новой выгрузки
// Формат: Выгрузка_<тип>_<конфигурация>_<компьютер>_<пользователь>_<время>.db
func generateDatabaseFileName(uploadType, configName, computerName, userName string) string {
	// Если тип данных неизвестен, используем "ПолнаяВыгрузка"
	if uploadType == "" {
		uploadType = "ПолнаяВыгрузка"
	}

	// Очищаем имена от недопустимых символов для файловой системы
	sanitize := func(s string) string {
		// Заменяем недопустимые символы на подчеркивания
		invalidChars := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|", " ", "\n", "\r", "\t"}
		result := s
		for _, char := range invalidChars {
			result = strings.ReplaceAll(result, char, "_")
		}
		// Удаляем множественные подчеркивания
		for strings.Contains(result, "__") {
			result = strings.ReplaceAll(result, "__", "_")
		}
		// Удаляем подчеркивания в начале и конце
		result = strings.Trim(result, "_")
		// Если строка пустая после очистки, используем значение по умолчанию
		if result == "" {
			result = "Unknown"
		}
		return result
	}

	uploadType = sanitize(uploadType)
	configName = sanitize(configName)
	computerName = sanitize(computerName)
	userName = sanitize(userName)

	// Если имена пустые, используем значения по умолчанию
	if configName == "" {
		configName = "Unknown"
	}
	if computerName == "" {
		computerName = "Unknown"
	}
	if userName == "" {
		userName = "Unknown"
	}

	// Формируем время в формате: 2024-01-15_14-30-25
	now := time.Now()
	timeStr := now.Format("2006-01-02_15-04-05")

	// Формируем имя файла
	fileName := fmt.Sprintf("Выгрузка_%s_%s_%s_%s_%s.db",
		uploadType, configName, computerName, userName, timeStr)

	return fileName
}

// getUploadDatabase получает БД для выгрузки из кэша или открывает её по пути
func (s *Server) getUploadDatabase(uploadUUID string) (*database.DB, error) {
	s.uploadDBsMutex.RLock()
	uploadDB, exists := s.uploadDBs[uploadUUID]
	s.uploadDBsMutex.RUnlock()

	if exists && uploadDB != nil {
		return uploadDB, nil
	}

	// Если БД нет в кэше, пытаемся найти её через service.db
	// Ищем все БД в service.db и проверяем каждую на наличие upload с таким UUID
	if s.serviceDB != nil {
		// Получаем все проекты
		clients, err := s.serviceDB.GetClientsWithStats()
		if err == nil {
			for _, clientMap := range clients {
				clientIDFloat, ok := clientMap["id"].(float64)
				if !ok {
					continue
				}
				clientID := int(clientIDFloat)
				projects, err := s.serviceDB.GetClientProjects(clientID)
				if err == nil {
					for _, project := range projects {
						databases, err := s.serviceDB.GetProjectDatabases(project.ID, false)
						if err == nil {
							for _, dbInfo := range databases {
								// Проверяем, существует ли файл БД
								if _, err := os.Stat(dbInfo.FilePath); err == nil {
									// Открываем БД и проверяем наличие upload
									tempDB, err := database.NewDB(dbInfo.FilePath)
									if err == nil {
										upload, err := tempDB.GetUploadByUUID(uploadUUID)
										tempDB.Close()
										if err == nil && upload != nil {
											// Нашли БД! Открываем её и сохраняем в кэш
											uploadDB, err := database.NewDBWithConfig(dbInfo.FilePath, database.DBConfig{
												MaxOpenConns:    s.config.MaxOpenConns,
												MaxIdleConns:    s.config.MaxIdleConns,
												ConnMaxLifetime: s.config.ConnMaxLifetime,
											})
											if err == nil {
												s.uploadDBsMutex.Lock()
												s.uploadDBs[uploadUUID] = uploadDB
												s.uploadDBsMutex.Unlock()
												return uploadDB, nil
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}

	return nil, fmt.Errorf("database for upload %s not found in cache or service.db", uploadUUID)
}

// handleHandshake обрабатывает рукопожатие
func (s *Server) handleHandshake(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.writeErrorResponse(w, "Failed to read request body", err)
		return
	}

	var req HandshakeRequest
	if err := xml.Unmarshal(body, &req); err != nil {
		s.writeErrorResponse(w, "Failed to parse XML", err)
		return
	}

	// Валидация обязательных полей
	if req.Version1C == "" {
		s.writeErrorResponse(w, "Missing required field: version_1c", fmt.Errorf("version_1c is required"))
		return
	}
	if req.ConfigName == "" {
		s.writeErrorResponse(w, "Missing required field: config_name", fmt.Errorf("config_name is required"))
		return
	}

	// Логирование всех полей итераций для отладки
	s.log(LogEntry{
		Timestamp: time.Now(),
		Level:     "DEBUG",
		Message: fmt.Sprintf("Handshake request received - Version1C: %s, ConfigName: %s, DatabaseID: %s, IterationNumber: %d, IterationLabel: %s, ProgrammerName: %s, UploadPurpose: %s, ParentUploadID: %s",
			req.Version1C, req.ConfigName, req.DatabaseID, req.IterationNumber, req.IterationLabel, req.ProgrammerName, req.UploadPurpose, req.ParentUploadID),
		Endpoint: "/handshake",
	})

	// Создаем новую выгрузку
	uploadUUID := uuid.New().String()

	// Определяем parent_upload_id если указан ParentUploadID (UUID) - ищем в текущей БД сервера
	var parentUploadID *int
	if req.ParentUploadID != "" {
		parentUpload, err := s.db.GetUploadByUUID(req.ParentUploadID)
		if err == nil {
			parentUploadID = &parentUpload.ID
		}
	}

	// Определяем database_id с приоритетами (ищем в текущей БД сервера):
	// 1. Прямой database_id из запроса (если указан)
	// 2. Автоматический поиск по косвенным параметрам (computer_name, user_name, config_name, version_1c)
	var databaseID *int
	var clientName, projectName string
	var identifiedBy string            // Для логирования способа идентификации
	var similarUpload *database.Upload // Для хранения похожей выгрузки

	if req.DatabaseID != "" {
		// Приоритет 1: Прямой database_id из запроса
		dbID, err := strconv.Atoi(req.DatabaseID)
		if err == nil {
			databaseID = &dbID
			identifiedBy = "direct_database_id"

			// Получаем информацию о базе данных из serviceDB
			if s.serviceDB != nil {
				dbInfo, err := s.serviceDB.GetProjectDatabase(dbID)
				if err == nil && dbInfo != nil {
					// Получаем информацию о проекте
					project, err := s.serviceDB.GetClientProject(dbInfo.ClientProjectID)
					if err == nil && project != nil {
						projectName = project.Name

						// Получаем информацию о клиенте
						client, err := s.serviceDB.GetClient(project.ClientID)
						if err == nil && client != nil {
							clientName = client.Name
						}
					}
				}
			}
		}
	} else {
		// Приоритет 2: Автоматический поиск по косвенным параметрам
		var err error
		similarUpload, err = s.db.FindSimilarUpload(
			req.ComputerName,
			req.UserName,
			req.ConfigName,
			req.Version1C,
			req.ConfigVersion,
		)

		if err == nil && similarUpload != nil && similarUpload.DatabaseID != nil {
			databaseID = similarUpload.DatabaseID
			identifiedBy = fmt.Sprintf("similar_upload_%d", similarUpload.ID)

			// Получаем информацию о базе данных из serviceDB
			if s.serviceDB != nil {
				dbInfo, err := s.serviceDB.GetProjectDatabase(*databaseID)
				if err == nil && dbInfo != nil {
					// Получаем информацию о проекте
					project, err := s.serviceDB.GetClientProject(dbInfo.ClientProjectID)
					if err == nil && project != nil {
						projectName = project.Name

						// Получаем информацию о клиенте
						client, err := s.serviceDB.GetClient(project.ClientID)
						if err == nil && client != nil {
							clientName = client.Name
						}
					}
				}
			}

			// Логируем успешную автоматическую идентификацию
			s.log(LogEntry{
				Timestamp: time.Now(),
				Level:     "INFO",
				Message: fmt.Sprintf("Auto-identified database_id=%d from similar upload (computer=%s, config=%s, version=%s)",
					*databaseID, req.ComputerName, req.ConfigName, req.Version1C),
				UploadUUID: uploadUUID,
				Endpoint:   "/handshake",
			})
		} else {
			identifiedBy = "none"
			// Логируем, что автоматическая идентификация не удалась
			s.log(LogEntry{
				Timestamp: time.Now(),
				Level:     "INFO",
				Message: fmt.Sprintf("Could not auto-identify database (computer=%s, config=%s, version=%s)",
					req.ComputerName, req.ConfigName, req.Version1C),
				UploadUUID: uploadUUID,
				Endpoint:   "/handshake",
			})
		}
	}

	// Определяем тип выгружаемых данных (для логирования)
	uploadType := req.UploadType
	if uploadType == "" {
		uploadType = "ПолнаяВыгрузка"
	}

	// Устанавливаем значения по умолчанию для полей итераций
	iterationNumber := req.IterationNumber
	if iterationNumber <= 0 {
		iterationNumber = 1
	}

	// НОВАЯ ЛОГИКА: Создаем выгрузку в ЕДИНОЙ БД (s.unifiedCatalogsDB)
	// Вместо создания отдельного файла БД для каждой выгрузки
	if s.unifiedCatalogsDB == nil {
		s.writeErrorResponse(w, "Unified catalogs database not initialized", fmt.Errorf("unifiedCatalogsDB is nil"))
		return
	}

	upload, err := s.unifiedCatalogsDB.CreateUploadWithDatabase(
		uploadUUID, req.Version1C, req.ConfigName, databaseID,
		req.ComputerName, req.UserName, req.ConfigVersion,
		iterationNumber, req.IterationLabel, req.ProgrammerName, req.UploadPurpose, parentUploadID,
	)
	if err != nil {
		s.writeErrorResponse(w, "Failed to create upload", err)
		return
	}

	// Сохраняем ссылку на единую БД в кэш (для совместимости с существующим кодом)
	s.uploadDBsMutex.Lock()
	s.uploadDBs[uploadUUID] = s.unifiedCatalogsDB
	s.uploadDBsMutex.Unlock()

	// Нет необходимости регистрировать новый файл БД в service.db,
	// так как теперь все данные в одной БД

	// Обновляем кэшированные значения client_id и project_id
	if databaseID != nil {
		var clientID, projectID int

		// Если идентификация была по похожей выгрузке, используем её значения
		if identifiedBy != "" && strings.HasPrefix(identifiedBy, "similar_upload_") {
			// Значения уже получены из similarUpload выше
			if similarUpload != nil {
				if similarUpload.ClientID != nil {
					clientID = *similarUpload.ClientID
				}
				if similarUpload.ProjectID != nil {
					projectID = *similarUpload.ProjectID
				}
			}
		}

		// Если не получили из похожей выгрузки, получаем из serviceDB
		if clientID == 0 || projectID == 0 {
			if s.serviceDB != nil && databaseID != nil {
				dbInfo, err := s.serviceDB.GetProjectDatabase(*databaseID)
				if err == nil && dbInfo != nil {
					project, err := s.serviceDB.GetClientProject(dbInfo.ClientProjectID)
					if err == nil && project != nil {
						clientID = project.ClientID
						projectID = project.ID
					}
				}
			}
		}

		// Обновляем upload с кэшированными значениями (в единой БД)
		if clientID > 0 && projectID > 0 {
			_, err = s.unifiedCatalogsDB.Exec(`
				UPDATE uploads 
				SET client_id = ?, project_id = ? 
				WHERE id = ?
			`, clientID, projectID, upload.ID)
			if err != nil {
				// Логируем ошибку, но не прерываем процесс
				s.log(LogEntry{
					Timestamp:  time.Now(),
					Level:      "WARNING",
					Message:    fmt.Sprintf("Failed to update cached client_id and project_id: %v", err),
					UploadUUID: uploadUUID,
					Endpoint:   "/handshake",
				})
			}
		}
	}

	s.log(LogEntry{
		Timestamp: time.Now(),
		Level:     "INFO",
		Message: fmt.Sprintf("Handshake successful for upload %s (unified_db, database_id: %v, identified_by: %s, iteration_number: %d, iteration_label: %s, programmer: %s, purpose: %s, parent_upload_id: %v, upload_type: %s)",
			uploadUUID, databaseID, identifiedBy, upload.IterationNumber, upload.IterationLabel, upload.ProgrammerName, upload.UploadPurpose, upload.ParentUploadID, uploadType),
		UploadUUID: uploadUUID,
		Endpoint:   "/handshake",
	})

	response := HandshakeResponse{
		Success:      true,
		UploadUUID:   uploadUUID,
		ClientName:   clientName,
		ProjectName:  projectName,
		DatabaseName: "unified_catalogs.db", // Теперь всегда единая БД
		DatabasePath: s.config.UnifiedCatalogsDBPath,
		DatabaseID:   0, // Нет отдельного database_id для файла
		Message:      "Handshake successful",
		Timestamp:    time.Now().Format(time.RFC3339),
	}

	s.writeXMLResponse(w, response)
}

// handleMetadata обрабатывает метаинформацию
func (s *Server) handleMetadata(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.writeErrorResponse(w, "Failed to read request body", err)
		return
	}

	var req MetadataRequest
	if err := xml.Unmarshal(body, &req); err != nil {
		s.writeErrorResponse(w, "Failed to parse XML", err)
		return
	}

	// Получаем БД для этой выгрузки
	uploadDB, err := s.getUploadDatabase(req.UploadUUID)
	if err != nil {
		s.writeErrorResponse(w, fmt.Sprintf("Failed to get upload database: %v", err), err)
		return
	}

	// Проверяем существование выгрузки
	_, err = uploadDB.GetUploadByUUID(req.UploadUUID)
	if err != nil {
		s.writeErrorResponse(w, "Upload not found", err)
		return
	}

	s.log(LogEntry{
		Timestamp:  time.Now(),
		Level:      "INFO",
		Message:    "Metadata received successfully",
		UploadUUID: req.UploadUUID,
		Endpoint:   "/metadata",
	})

	response := MetadataResponse{
		Success:   true,
		Message:   "Metadata received successfully",
		Timestamp: time.Now().Format(time.RFC3339),
	}

	s.writeXMLResponse(w, response)
}

// handleConstant обрабатывает константу
func (s *Server) handleConstant(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.writeErrorResponse(w, "Failed to read request body", err)
		return
	}

	// Логирование сырого XML для отладки
	bodyStr := string(body)
	bodyPreview := bodyStr
	if len(bodyPreview) > 500 {
		bodyPreview = bodyPreview[:500] + "..."
	}
	s.log(LogEntry{
		Timestamp: time.Now(),
		Level:     "DEBUG",
		Message:   fmt.Sprintf("Received constant XML (length: %d): %s", len(bodyStr), bodyPreview),
		Endpoint:  "/constant",
	})

	var req ConstantRequest
	if err := xml.Unmarshal(body, &req); err != nil {
		s.log(LogEntry{
			Timestamp: time.Now(),
			Level:     "ERROR",
			Message:   fmt.Sprintf("Failed to parse XML: %v, body preview: %s", err, bodyPreview),
			Endpoint:  "/constant",
		})
		s.writeErrorResponse(w, "Failed to parse XML", err)
		return
	}

	// Логирование распарсенных данных
	s.log(LogEntry{
		Timestamp: time.Now(),
		Level:     "DEBUG",
		Message:   fmt.Sprintf("Parsed constant - Name: %s, Type: %s, Value.Content length: %d, Value.Content: %s", req.Name, req.Type, len(req.Value.Content), req.Value.Content),
		Endpoint:  "/constant",
	})

	// Получаем БД для этой выгрузки
	uploadDB, err := s.getUploadDatabase(req.UploadUUID)
	if err != nil {
		s.writeErrorResponse(w, fmt.Sprintf("Failed to get upload database: %v", err), err)
		return
	}

	// Получаем выгрузку из БД upload
	upload, err := uploadDB.GetUploadByUUID(req.UploadUUID)
	if err != nil {
		s.writeErrorResponse(w, "Upload not found", err)
		return
	}

	// Добавляем константу
	// req.Value теперь структура ConstantValue, используем Content для получения XML строки
	valueContent := req.Value.Content
	if err := uploadDB.AddConstant(upload.ID, req.Name, req.Synonym, req.Type, valueContent); err != nil {
		s.writeErrorResponse(w, "Failed to add constant", err)
		return
	}

	// Логирование для отладки (логируем первые 100 символов значения)
	valuePreview := valueContent
	if len(valuePreview) > 100 {
		valuePreview = valuePreview[:100] + "..."
	}

	s.log(LogEntry{
		Timestamp:  time.Now(),
		Level:      "INFO",
		Message:    fmt.Sprintf("Constant '%s' (type: %s) added successfully, value preview: %s", req.Name, req.Type, valuePreview),
		UploadUUID: req.UploadUUID,
		Endpoint:   "/constant",
	})

	response := ConstantResponse{
		Success:   true,
		Message:   "Constant added successfully",
		Timestamp: time.Now().Format(time.RFC3339),
	}

	s.writeXMLResponse(w, response)
}

// handleCatalogMeta обрабатывает метаданные справочника
func (s *Server) handleCatalogMeta(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.writeErrorResponse(w, "Failed to read request body", err)
		return
	}

	var req CatalogMetaRequest
	if err := xml.Unmarshal(body, &req); err != nil {
		s.writeErrorResponse(w, "Failed to parse XML", err)
		return
	}

	// Получаем БД для этой выгрузки (теперь всегда единая БД)
	uploadDB, err := s.getUploadDatabase(req.UploadUUID)
	if err != nil {
		s.writeErrorResponse(w, fmt.Sprintf("Failed to get upload database: %v", err), err)
		return
	}

	// Получаем выгрузку из БД upload
	upload, err := uploadDB.GetUploadByUUID(req.UploadUUID)
	if err != nil {
		s.writeErrorResponse(w, "Upload not found", err)
		return
	}

	// НОВАЯ ЛОГИКА: Получаем или создаём таблицу для этого справочника
	tableName, err := database.GetOrCreateCatalogTable(uploadDB.GetDB(), req.Name)
	if err != nil {
		s.writeErrorResponse(w, fmt.Sprintf("Failed to create catalog table: %v", err), err)
		return
	}

	log.Printf("✓ Таблица для справочника '%s' готова: %s", req.Name, tableName)

	// Сохраняем маппинг (если ещё не сохранён)
	// GetOrCreateCatalogTable уже сохраняет маппинг внутри

	// Обновляем счётчик справочников
	_, err = uploadDB.Exec("UPDATE uploads SET total_catalogs = total_catalogs + 1 WHERE id = ?", upload.ID)
	if err != nil {
		log.Printf("Warning: Failed to update catalogs counter: %v", err)
	}

	s.log(LogEntry{
		Timestamp:  time.Now(),
		Level:      "INFO",
		Message:    fmt.Sprintf("Catalog '%s' metadata added (table: %s)", req.Name, tableName),
		UploadUUID: req.UploadUUID,
		Endpoint:   "/catalog/meta",
	})

	response := CatalogMetaResponse{
		Success:   true,
		CatalogID: upload.ID, // Возвращаем upload_id как catalog_id для обратной совместимости
		Message:   "Catalog metadata added successfully",
		Timestamp: time.Now().Format(time.RFC3339),
	}

	s.writeXMLResponse(w, response)
}

// handleCatalogItem обрабатывает элемент справочника
func (s *Server) handleCatalogItem(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.writeErrorResponse(w, "Failed to read request body", err)
		return
	}

	// ОТЛАДКА: Логируем входящий запрос
	log.Printf("[DEBUG] ========================================")
	log.Printf("[DEBUG] ОБРАБОТКА /catalog/item")
	log.Printf("[DEBUG] ========================================")
	log.Printf("[DEBUG] Размер тела запроса: %d байт", len(body))
	if len(body) > 0 {
		bodyPreview := string(body)
		if len(bodyPreview) > 2000 {
			log.Printf("[DEBUG] Тело запроса (первые 2000 символов):\n%s...", bodyPreview[:2000])
		} else {
			log.Printf("[DEBUG] Полное тело запроса:\n%s", bodyPreview)
		}
	} else {
		log.Printf("[DEBUG] ⚠ ВНИМАНИЕ: Тело запроса пустое!")
	}

	var req CatalogItemRequest
	if err := xml.Unmarshal(body, &req); err != nil {
		log.Printf("[DEBUG] ✗ ОШИБКА парсинга XML: %v", err)
		s.writeErrorResponse(w, "Failed to parse XML", err)
		return
	}

	// ОТЛАДКА: Логируем распарсенные данные
	log.Printf("[DEBUG] --- Распарсенные данные ---")
	log.Printf("[DEBUG] UploadUUID: %s", req.UploadUUID)
	log.Printf("[DEBUG] CatalogName: %s", req.CatalogName)
	log.Printf("[DEBUG] Reference: %s", req.Reference)
	log.Printf("[DEBUG] Code: %s", req.Code)
	log.Printf("[DEBUG] Name: %s", req.Name)
	attrsContent := req.Attributes.Content
	log.Printf("[DEBUG] Attributes длина: %d символов", len(attrsContent))
	if len(attrsContent) > 0 {
		attrsPreview := attrsContent
		if len(attrsPreview) > 1000 {
			log.Printf("[DEBUG] Attributes (первые 1000 символов):\n%s...", attrsPreview[:1000])
		} else {
			log.Printf("[DEBUG] Полное содержимое Attributes:\n%s", attrsPreview)
		}
		// Подсчитываем количество элементов <Реквизит>
		attrsCount := strings.Count(attrsContent, "<Реквизит")
		log.Printf("[DEBUG] Найдено элементов <Реквизит>: %d", attrsCount)
	} else {
		log.Printf("[DEBUG] ⚠ ВНИМАНИЕ: Attributes ПУСТОЙ!")
	}
	tablePartsContent := req.TableParts.Content
	log.Printf("[DEBUG] TableParts длина: %d символов", len(tablePartsContent))
	if len(tablePartsContent) > 0 {
		partsPreview := tablePartsContent
		if len(partsPreview) > 500 {
			log.Printf("[DEBUG] TableParts (первые 500 символов):\n%s...", partsPreview[:500])
		} else {
			log.Printf("[DEBUG] Полное содержимое TableParts:\n%s", partsPreview)
		}
	}
	log.Printf("[DEBUG] Timestamp: %s", req.Timestamp)

	// Получаем БД для этой выгрузки (теперь всегда единая БД)
	uploadDB, err := s.getUploadDatabase(req.UploadUUID)
	if err != nil {
		s.writeErrorResponse(w, fmt.Sprintf("Failed to get upload database: %v", err), err)
		return
	}

	// Получаем выгрузку из БД upload
	upload, err := uploadDB.GetUploadByUUID(req.UploadUUID)
	if err != nil {
		s.writeErrorResponse(w, "Upload not found", err)
		return
	}

	// НОВАЯ ЛОГИКА: Получаем имя таблицы для этого справочника
	tableName, err := database.GetCatalogTableName(uploadDB.GetDB(), req.CatalogName)
	if err != nil {
		// Если таблица не найдена, создаём её (на случай если пропустили /catalog/meta)
		tableName, err = database.GetOrCreateCatalogTable(uploadDB.GetDB(), req.CatalogName)
		if err != nil {
			s.writeErrorResponse(w, fmt.Sprintf("Failed to get/create catalog table: %v", err), err)
			return
		}
		log.Printf("[DEBUG] Таблица для справочника '%s' создана автоматически: %s", req.CatalogName, tableName)
	}

	log.Printf("[DEBUG] Используется таблица: %s", tableName)

	// Attributes и TableParts уже приходят как XML строки из 1С
	// Передаем их напрямую как строки
	log.Printf("[DEBUG] --- Сохранение в БД ---")
	log.Printf("[DEBUG] tableName: %s", tableName)
	log.Printf("[DEBUG] upload_id: %d", upload.ID)
	log.Printf("[DEBUG] reference: %s", req.Reference)
	log.Printf("[DEBUG] code: %s", req.Code)
	log.Printf("[DEBUG] name: %s", req.Name)
	attrsStr := req.Attributes.Content
	tablePartsStr := req.TableParts.Content
	log.Printf("[DEBUG] attributes для сохранения (длина: %d): %s", len(attrsStr), 
		func() string {
			if len(attrsStr) > 500 {
				return attrsStr[:500] + "..."
			}
			return attrsStr
		}())
	log.Printf("[DEBUG] tableParts для сохранения (длина: %d)", len(tablePartsStr))
	
	// Используем новую функцию для вставки в динамическую таблицу
	if err := uploadDB.AddCatalogItemToTable(tableName, upload.ID, req.Reference, req.Code, req.Name, attrsStr, tablePartsStr); err != nil {
		log.Printf("[DEBUG] ✗ ОШИБКА при сохранении в БД: %v", err)
		s.writeErrorResponse(w, "Failed to add catalog item", err)
		return
	}

	log.Printf("[DEBUG] ✓ Элемент успешно сохранен в таблицу %s", tableName)
	log.Printf("[DEBUG] ========================================")

	s.log(LogEntry{
		Timestamp:  time.Now(),
		Level:      "INFO",
		Message:    fmt.Sprintf("Catalog item '%s' added to table %s", req.Name, tableName),
		UploadUUID: req.UploadUUID,
		Endpoint:   "/catalog/item",
	})

	response := CatalogItemResponse{
		Success:   true,
		Message:   "Catalog item added successfully",
		Timestamp: time.Now().Format(time.RFC3339),
	}

	s.writeXMLResponse(w, response)
}

// handleCatalogItems обрабатывает пакетную загрузку элементов справочника
func (s *Server) handleCatalogItems(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.writeErrorResponse(w, "Failed to read request body", err)
		return
	}

	// ОТЛАДКА: Логируем входящий запрос
	log.Printf("[DEBUG] ========================================")
	log.Printf("[DEBUG] ОБРАБОТКА /catalog/items (ПАКЕТ)")
	log.Printf("[DEBUG] ========================================")
	log.Printf("[DEBUG] Размер тела запроса: %d байт", len(body))
	if len(body) > 0 {
		bodyPreview := string(body)
		if len(bodyPreview) > 3000 {
			log.Printf("[DEBUG] Тело запроса (первые 3000 символов):\n%s...", bodyPreview[:3000])
		} else {
			log.Printf("[DEBUG] Полное тело запроса:\n%s", bodyPreview)
		}
	} else {
		log.Printf("[DEBUG] ⚠ ВНИМАНИЕ: Тело запроса пустое!")
	}

	var req CatalogItemsRequest
	if err := xml.Unmarshal(body, &req); err != nil {
		log.Printf("[DEBUG] ✗ ОШИБКА парсинга XML: %v", err)
		s.writeErrorResponse(w, "Failed to parse XML", err)
		return
	}

	// ОТЛАДКА: Логируем распарсенные данные
	log.Printf("[DEBUG] --- Распарсенные данные пакета ---")
	log.Printf("[DEBUG] UploadUUID: %s", req.UploadUUID)
	log.Printf("[DEBUG] CatalogName: %s", req.CatalogName)
	log.Printf("[DEBUG] Количество элементов в пакете: %d", len(req.Items))
	
	// Детали первых 3 элементов
	for i, item := range req.Items {
		if i < 3 {
			log.Printf("[DEBUG] --- Элемент #%d ---", i+1)
			log.Printf("[DEBUG]   Reference: %s", item.Reference)
			log.Printf("[DEBUG]   Code: %s", item.Code)
			log.Printf("[DEBUG]   Name: %s", item.Name)
			itemAttrsContent := item.Attributes.Content
			log.Printf("[DEBUG]   Attributes длина: %d символов", len(itemAttrsContent))
			if len(itemAttrsContent) > 0 {
				attrsPreview := itemAttrsContent
				if len(attrsPreview) > 500 {
					log.Printf("[DEBUG]   Attributes (первые 500 символов):\n%s...", attrsPreview[:500])
				} else {
					log.Printf("[DEBUG]   Полное содержимое Attributes:\n%s", attrsPreview)
				}
				attrsCount := strings.Count(itemAttrsContent, "<Реквизит")
				log.Printf("[DEBUG]   Найдено элементов <Реквизит>: %d", attrsCount)
			} else {
				log.Printf("[DEBUG]   ⚠ ВНИМАНИЕ: Attributes ПУСТОЙ!")
			}
			itemTablePartsContent := item.TableParts.Content
			log.Printf("[DEBUG]   TableParts длина: %d символов", len(itemTablePartsContent))
		}
	}

	// Получаем БД для этой выгрузки
	uploadDB, err := s.getUploadDatabase(req.UploadUUID)
	if err != nil {
		s.writeErrorResponse(w, fmt.Sprintf("Failed to get upload database: %v", err), err)
		return
	}

	// Получаем выгрузку из БД upload
	upload, err := uploadDB.GetUploadByUUID(req.UploadUUID)
	if err != nil {
		s.writeErrorResponse(w, "Upload not found", err)
		return
	}

	// Находим справочник по имени
	var catalogID int
	err = uploadDB.QueryRow("SELECT id FROM catalogs WHERE upload_id = ? AND name = ?", upload.ID, req.CatalogName).Scan(&catalogID)
	if err != nil {
		s.writeErrorResponse(w, "Catalog not found", err)
		return
	}

	// Обрабатываем каждый элемент пакета
	processedCount := 0
	failedCount := 0
	itemsWithAttrs := 0
	itemsWithoutAttrs := 0

	log.Printf("[DEBUG] --- Сохранение элементов в БД ---")
	log.Printf("[DEBUG] catalogID: %d", catalogID)
	
	for i, item := range req.Items {
		itemAttrsStr := item.Attributes.Content
		itemTablePartsStr := item.TableParts.Content
		
		// ОТЛАДКА: Детали сохранения первых 3 элементов
		if i < 3 {
			log.Printf("[DEBUG] --- Сохранение элемента #%d ---", i+1)
			log.Printf("[DEBUG]   Code: %s, Name: %s", item.Code, item.Name)
			log.Printf("[DEBUG]   Attributes для сохранения (длина: %d)", len(itemAttrsStr))
			if len(itemAttrsStr) > 0 {
				itemsWithAttrs++
				if len(itemAttrsStr) > 500 {
					log.Printf("[DEBUG]   Attributes (первые 500 символов):\n%s...", itemAttrsStr[:500])
				} else {
					log.Printf("[DEBUG]   Полное содержимое Attributes:\n%s", itemAttrsStr)
				}
			} else {
				itemsWithoutAttrs++
				log.Printf("[DEBUG]   ⚠ Attributes ПУСТОЙ!")
			}
		} else {
			// Для остальных элементов просто считаем
			if len(itemAttrsStr) > 0 {
				itemsWithAttrs++
			} else {
				itemsWithoutAttrs++
			}
		}
		
		if err := uploadDB.AddCatalogItem(catalogID, item.Reference, item.Code, item.Name, itemAttrsStr, itemTablePartsStr); err != nil {
			failedCount++
			log.Printf("[DEBUG]   ✗ ОШИБКА при сохранении элемента #%d: %v", i+1, err)
			s.log(LogEntry{
				Timestamp:  time.Now(),
				Level:      "ERROR",
				Message:    fmt.Sprintf("Failed to add catalog item '%s': %v", item.Name, err),
				UploadUUID: req.UploadUUID,
				Endpoint:   "/catalog/items",
			})
		} else {
			processedCount++
			if i < 3 {
				log.Printf("[DEBUG]   ✓ Элемент #%d успешно сохранен", i+1)
			}
		}
	}
	
	log.Printf("[DEBUG] --- Статистика пакета ---")
	log.Printf("[DEBUG] Всего элементов: %d", len(req.Items))
	log.Printf("[DEBUG] С атрибутами: %d", itemsWithAttrs)
	log.Printf("[DEBUG] Без атрибутов: %d", itemsWithoutAttrs)
	log.Printf("[DEBUG] Успешно сохранено: %d", processedCount)
	log.Printf("[DEBUG] Ошибок: %d", failedCount)
	log.Printf("[DEBUG] ========================================")

	s.log(LogEntry{
		Timestamp:  time.Now(),
		Level:      "INFO",
		Message:    fmt.Sprintf("Batch catalog items processed: %d successful, %d failed", processedCount, failedCount),
		UploadUUID: req.UploadUUID,
		Endpoint:   "/catalog/items",
	})

	response := CatalogItemsResponse{
		Success:        true,
		ProcessedCount: processedCount,
		FailedCount:    failedCount,
		Message:        fmt.Sprintf("Processed %d items, %d failed", processedCount, failedCount),
		Timestamp:      time.Now().Format(time.RFC3339),
	}

	s.writeXMLResponse(w, response)
}

// handleNomenclatureBatch обрабатывает пакетную загрузку номенклатуры с характеристиками
func (s *Server) handleNomenclatureBatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.writeErrorResponse(w, "Failed to read request body", err)
		return
	}

	var req NomenclatureBatchRequest
	if err := xml.Unmarshal(body, &req); err != nil {
		s.writeErrorResponse(w, "Failed to parse XML", err)
		return
	}

	// Получаем БД для этой выгрузки
	uploadDB, err := s.getUploadDatabase(req.UploadUUID)
	if err != nil {
		s.writeErrorResponse(w, fmt.Sprintf("Failed to get upload database: %v", err), err)
		return
	}

	// Получаем выгрузку из БД upload
	upload, err := uploadDB.GetUploadByUUID(req.UploadUUID)
	if err != nil {
		s.writeErrorResponse(w, "Upload not found", err)
		return
	}

	// Преобразуем элементы в формат для базы данных
	nomenclatureItems := make([]database.NomenclatureItem, 0, len(req.Items))
	for _, item := range req.Items {
		nomenclatureItems = append(nomenclatureItems, database.NomenclatureItem{
			NomenclatureReference:   item.NomenclatureReference,
			NomenclatureCode:        item.NomenclatureCode,
			NomenclatureName:        item.NomenclatureName,
			CharacteristicReference: item.CharacteristicReference,
			CharacteristicName:      item.CharacteristicName,
			AttributesXML:           item.Attributes,
			TablePartsXML:           item.TableParts,
		})
	}

	// Добавляем пакет элементов номенклатуры
	if err := uploadDB.AddNomenclatureItemsBatch(upload.ID, nomenclatureItems); err != nil {
		s.writeErrorResponse(w, "Failed to add nomenclature items", err)
		return
	}

	processedCount := len(req.Items)

	s.log(LogEntry{
		Timestamp:  time.Now(),
		Level:      "INFO",
		Message:    fmt.Sprintf("Batch nomenclature items processed: %d items", processedCount),
		UploadUUID: req.UploadUUID,
		Endpoint:   "/api/v1/upload/nomenclature/batch",
	})

	response := NomenclatureBatchResponse{
		Success:        true,
		ProcessedCount: processedCount,
		FailedCount:    0,
		Message:        fmt.Sprintf("Processed %d nomenclature items", processedCount),
		Timestamp:      time.Now().Format(time.RFC3339),
	}

	s.writeXMLResponse(w, response)
}

// handleComplete обрабатывает завершение выгрузки
func (s *Server) handleComplete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.writeErrorResponse(w, "Failed to read request body", err)
		return
	}

	var req CompleteRequest
	if err := xml.Unmarshal(body, &req); err != nil {
		s.writeErrorResponse(w, "Failed to parse XML", err)
		return
	}

	// Получаем БД для этой выгрузки
	uploadDB, err := s.getUploadDatabase(req.UploadUUID)
	if err != nil {
		s.writeErrorResponse(w, fmt.Sprintf("Failed to get upload database: %v", err), err)
		return
	}

	// Получаем выгрузку из БД upload
	upload, err := uploadDB.GetUploadByUUID(req.UploadUUID)
	if err != nil {
		s.writeErrorResponse(w, "Upload not found", err)
		return
	}

	// Завершаем выгрузку
	if err := uploadDB.CompleteUpload(upload.ID); err != nil {
		s.writeErrorResponse(w, "Failed to complete upload", err)
		return
	}

	s.log(LogEntry{
		Timestamp:  time.Now(),
		Level:      "INFO",
		Message:    fmt.Sprintf("Upload %s completed successfully", req.UploadUUID),
		UploadUUID: req.UploadUUID,
		Endpoint:   "/complete",
	})

	// Запускаем анализ качества в фоне
	go func() {
		databaseID := 0
		if upload.DatabaseID != nil {
			databaseID = *upload.DatabaseID
		}

		if databaseID > 0 {
			log.Printf("Starting quality analysis for upload %s (ID: %d, Database: %d)", req.UploadUUID, upload.ID, databaseID)
			if err := s.qualityAnalyzer.AnalyzeUpload(upload.ID, databaseID); err != nil {
				log.Printf("Quality analysis failed for upload %s: %v", req.UploadUUID, err)
			} else {
				log.Printf("Quality analysis completed for upload %s", req.UploadUUID)
			}
		} else {
			log.Printf("Skipping quality analysis for upload %s: database_id not set", req.UploadUUID)
		}
	}()

	response := CompleteResponse{
		Success:   true,
		Message:   "Upload completed successfully",
		Timestamp: time.Now().Format(time.RFC3339),
	}

	s.writeXMLResponse(w, response)
}

// handleStats обрабатывает запрос статистики
func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	stats, err := s.db.GetStats()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// handleHealth обрабатывает проверку здоровья сервера
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "healthy",
		"time":   time.Now().Format(time.RFC3339),
	})
}

// handleDatabaseV1Routes обрабатывает маршруты /api/v1/databases/{id}
func (s *Server) handleDatabaseV1Routes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/databases/")

	if path == "" {
		http.Error(w, "Database ID required", http.StatusBadRequest)
		return
	}

	// Парсим database_id (может быть строкой или числом)
	databaseID, err := strconv.Atoi(path)
	if err != nil {
		s.writeErrorResponse(w, "Invalid database ID", err)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Получаем информацию о базе данных
	if s.serviceDB == nil {
		s.writeErrorResponse(w, "Service database not available", fmt.Errorf("serviceDB is nil"))
		return
	}

	dbInfo, err := s.serviceDB.GetProjectDatabase(databaseID)
	if err != nil {
		s.writeErrorResponse(w, "Failed to get database", err)
		return
	}

	if dbInfo == nil {
		s.writeErrorResponse(w, "Database not found", fmt.Errorf("database with ID %d not found", databaseID))
		return
	}

	// Получаем информацию о проекте
	project, err := s.serviceDB.GetClientProject(dbInfo.ClientProjectID)
	if err != nil {
		s.writeErrorResponse(w, "Failed to get project", err)
		return
	}

	// Получаем информацию о клиенте
	client, err := s.serviceDB.GetClient(project.ClientID)
	if err != nil {
		s.writeErrorResponse(w, "Failed to get client", err)
		return
	}

	// Формируем XML ответ
	response := DatabaseInfoResponse{
		XMLName:      xml.Name{Local: "database_info"},
		DatabaseID:   strconv.Itoa(databaseID),
		DatabaseName: dbInfo.Name,
		ProjectID:    strconv.Itoa(project.ID),
		ProjectName:  project.Name,
		ClientID:     strconv.Itoa(client.ID),
		ClientName:   client.Name,
		Status:       "success",
		Message:      "Database information retrieved successfully",
		Timestamp:    time.Now().Format(time.RFC3339),
	}

	s.writeXMLResponse(w, response)
}

// DatabaseInfoResponse структура для ответа с информацией о базе данных
type DatabaseInfoResponse struct {
	XMLName      xml.Name `xml:"database_info"`
	DatabaseID   string   `xml:"database_id"`
	DatabaseName string   `xml:"database_name"`
	ProjectID    string   `xml:"project_id,omitempty"`
	ProjectName  string   `xml:"project_name"`
	ClientID     string   `xml:"client_id,omitempty"`
	ClientName   string   `xml:"client_name"`
	Status       string   `xml:"status"`
	Message      string   `xml:"message,omitempty"`
	Timestamp    string   `xml:"timestamp"`
}

// GetLogChannel возвращает канал для получения логов
func (s *Server) GetLogChannel() <-chan LogEntry {
	return s.logChan
}

// GetCircuitBreakerState возвращает состояние Circuit Breaker
func (s *Server) GetCircuitBreakerState() map[string]interface{} {
	if s.normalizer == nil || s.normalizer.GetAINormalizer() == nil {
		return map[string]interface{}{
			"enabled":       false,
			"state":         "unknown",
			"can_proceed":   false,
			"failure_count": 0,
		}
	}

	aiNormalizer := s.normalizer.GetAINormalizer()
	if aiNormalizer == nil {
		return map[string]interface{}{
			"enabled":       false,
			"state":         "unknown",
			"can_proceed":   false,
			"failure_count": 0,
		}
	}

	// Получаем реальное состояние Circuit Breaker через AINormalizer
	cbState := aiNormalizer.GetCircuitBreakerState()
	return cbState
}

// GetBatchProcessorStats возвращает статистику батчевой обработки
func (s *Server) GetBatchProcessorStats() map[string]interface{} {
	if s.normalizer == nil || s.normalizer.GetAINormalizer() == nil {
		return map[string]interface{}{
			"enabled":             false,
			"queue_size":          0,
			"total_batches":       0,
			"avg_items_per_batch": 0.0,
			"api_calls_saved":     0,
		}
	}

	aiNormalizer := s.normalizer.GetAINormalizer()
	if aiNormalizer == nil {
		return map[string]interface{}{
			"enabled":             false,
			"queue_size":          0,
			"total_batches":       0,
			"avg_items_per_batch": 0.0,
			"api_calls_saved":     0,
		}
	}

	// Получаем реальную статистику от BatchProcessor
	return aiNormalizer.GetBatchProcessorStats()
}

// GetCheckpointStatus возвращает статус checkpoint system
func (s *Server) GetCheckpointStatus() map[string]interface{} {
	if s.normalizer == nil {
		return map[string]interface{}{
			"enabled":          false,
			"active":           false,
			"processed_count":  0,
			"total_count":      0,
			"progress_percent": 0.0,
		}
	}

	// Получаем реальный статус от Normalizer
	return s.normalizer.GetCheckpointStatus()
}

// CollectMetricsSnapshot собирает текущий снимок метрик производительности
func (s *Server) CollectMetricsSnapshot() *database.PerformanceMetricsSnapshot {
	// Рассчитываем uptime
	uptime := time.Since(s.startTime).Seconds()

	// Получаем метрики от компонентов
	cbState := s.GetCircuitBreakerState()
	batchStats := s.GetBatchProcessorStats()
	checkpointStatus := s.GetCheckpointStatus()

	// Собираем AI и cache метрики
	aiSuccessRate := 0.0
	cacheHitRate := 0.0
	throughput := 0.0

	if s.normalizer != nil && s.normalizer.GetAINormalizer() != nil {
		statsCollector := s.normalizer.GetAINormalizer().GetStatsCollector()
		if statsCollector != nil {
			perfMetrics := statsCollector.GetMetrics()
			if perfMetrics.TotalAIRequests > 0 {
				aiSuccessRate = float64(perfMetrics.SuccessfulAIRequest) / float64(perfMetrics.TotalAIRequests)
			}
			if perfMetrics.TotalNormalized > 0 && uptime > 0 {
				throughput = float64(perfMetrics.TotalNormalized) / uptime
			}
		}

		cacheStats := s.normalizer.GetAINormalizer().GetCacheStats()
		cacheHitRate = cacheStats.HitRate
	}

	// Получаем checkpoint progress из checkpointStatus
	checkpointProgress := 0.0
	if progress, ok := checkpointStatus["progress_percent"].(float64); ok {
		checkpointProgress = progress
	}

	// Создаем snapshot с минимальными данными
	snapshot := &database.PerformanceMetricsSnapshot{
		Timestamp:           time.Now(),
		MetricType:          "all", // Общие метрики
		MetricData:          "",    // TODO: JSON с детальными метриками
		UptimeSeconds:       int(uptime),
		Throughput:          throughput,
		AISuccessRate:       aiSuccessRate,
		CacheHitRate:        cacheHitRate,
		CircuitBreakerState: cbState["state"].(string),
		CheckpointProgress:  checkpointProgress,
	}

	// Добавляем batch queue size если доступен
	if queueSize, ok := batchStats["queue_size"].(int); ok {
		snapshot.BatchQueueSize = queueSize
	}

	return snapshot
}

// writeJSONResponse записывает JSON ответ
func (s *Server) writeJSONResponse(w http.ResponseWriter, data interface{}, statusCode int) {
	middleware.WriteJSONResponse(w, data, statusCode)
}

// writeJSONError записывает JSON ошибку
func (s *Server) writeJSONError(w http.ResponseWriter, message string, statusCode int) {
	middleware.WriteJSONError(w, message, statusCode)
}

// handleListUploads обрабатывает запрос списка выгрузок
func (s *Server) handleListUploads(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	uploads, err := s.db.GetAllUploads()
	if err != nil {
		s.writeJSONError(w, fmt.Sprintf("Failed to get uploads: %v", err), http.StatusInternalServerError)
		return
	}

	items := make([]UploadListItem, len(uploads))
	for i, upload := range uploads {
		items[i] = UploadListItem{
			UploadUUID:     upload.UploadUUID,
			StartedAt:      upload.StartedAt,
			CompletedAt:    upload.CompletedAt,
			Status:         upload.Status,
			Version1C:      upload.Version1C,
			ConfigName:     upload.ConfigName,
			TotalConstants: upload.TotalConstants,
			TotalCatalogs:  upload.TotalCatalogs,
			TotalItems:     upload.TotalItems,
		}
	}

	s.log(LogEntry{
		Timestamp: time.Now(),
		Level:     "INFO",
		Message:   fmt.Sprintf("List uploads requested, returned %d uploads", len(items)),
		Endpoint:  "/api/uploads",
	})

	s.writeJSONResponse(w, map[string]interface{}{
		"uploads": items,
		"total":   len(items),
	}, http.StatusOK)
}

// handleUploadRoutes обрабатывает маршруты с UUID выгрузки
func (s *Server) handleUploadRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/uploads/")
	parts := strings.Split(path, "/")

	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}

	uuid := parts[0]

	// Получаем БД для этой выгрузки
	uploadDB, err := s.getUploadDatabase(uuid)
	if err != nil {
		s.writeJSONError(w, fmt.Sprintf("Upload database not found: %v", err), http.StatusNotFound)
		return
	}

	// Получаем выгрузку из БД upload
	upload, err := uploadDB.GetUploadByUUID(uuid)
	if err != nil {
		s.writeJSONError(w, "Upload not found", http.StatusNotFound)
		return
	}

	// Обрабатываем подмаршруты
	if len(parts) == 1 {
		// GET /api/uploads/{uuid} - детали выгрузки
		s.handleGetUpload(w, r, upload)
	} else if len(parts) == 2 {
		switch parts[1] {
		case "data":
			// GET /api/uploads/{uuid}/data - получение данных
			s.handleGetUploadData(w, r, upload)
		case "stream":
			// GET /api/uploads/{uuid}/stream - потоковая отправка
			s.handleStreamUploadData(w, r, upload)
		case "verify":
			// POST /api/uploads/{uuid}/verify - проверка передачи
			s.handleVerifyUpload(w, r, upload)
		case "export":
			// POST /api/uploads/{uuid}/export - запуск обратной выгрузки
			s.handleUploadExport(w, r, upload)
		case "exports":
			// GET /api/uploads/{uuid}/exports - список задач экспорта
			s.handleUploadExportsList(w, r, upload)
		default:
			http.NotFound(w, r)
		}
	} else {
		http.NotFound(w, r)
	}
}

// handleNormalizedListUploads обрабатывает запрос списка выгрузок из нормализованной БД
func (s *Server) handleNormalizedListUploads(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	uploads, err := s.normalizedDB.GetAllUploads()
	if err != nil {
		s.writeJSONError(w, fmt.Sprintf("Failed to get normalized uploads: %v", err), http.StatusInternalServerError)
		return
	}

	items := make([]UploadListItem, len(uploads))
	for i, upload := range uploads {
		items[i] = UploadListItem{
			UploadUUID:     upload.UploadUUID,
			StartedAt:      upload.StartedAt,
			CompletedAt:    upload.CompletedAt,
			Status:         upload.Status,
			Version1C:      upload.Version1C,
			ConfigName:     upload.ConfigName,
			TotalConstants: upload.TotalConstants,
			TotalCatalogs:  upload.TotalCatalogs,
			TotalItems:     upload.TotalItems,
		}
	}

	s.log(LogEntry{
		Timestamp: time.Now(),
		Level:     "INFO",
		Message:   fmt.Sprintf("List normalized uploads requested, returned %d uploads", len(items)),
		Endpoint:  "/api/normalized/uploads",
	})

	s.writeJSONResponse(w, map[string]interface{}{
		"uploads": items,
		"total":   len(items),
	}, http.StatusOK)
}

// handleNormalizedUploadRoutes обрабатывает маршруты с UUID выгрузки из нормализованной БД
func (s *Server) handleNormalizedUploadRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/normalized/uploads/")
	parts := strings.Split(path, "/")

	if len(parts) == 0 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}

	uuid := parts[0]

	// Проверяем существование выгрузки в нормализованной БД
	upload, err := s.normalizedDB.GetUploadByUUID(uuid)
	if err != nil {
		s.writeJSONError(w, "Normalized upload not found", http.StatusNotFound)
		return
	}

	// Обрабатываем подмаршруты
	if len(parts) == 1 {
		// GET /api/normalized/uploads/{uuid} - детали выгрузки
		s.handleGetUploadNormalized(w, r, upload)
	} else if len(parts) == 2 {
		switch parts[1] {
		case "data":
			// GET /api/normalized/uploads/{uuid}/data - получение данных
			s.handleGetUploadDataNormalized(w, r, upload)
		case "stream":
			// GET /api/normalized/uploads/{uuid}/stream - потоковая отправка
			s.handleStreamUploadDataNormalized(w, r, upload)
		case "verify":
			// POST /api/normalized/uploads/{uuid}/verify - проверка передачи
			s.handleVerifyUploadNormalized(w, r, upload)
		default:
			http.NotFound(w, r)
		}
	} else {
		http.NotFound(w, r)
	}
}

// handleGetUpload обрабатывает запрос детальной информации о выгрузке
func (s *Server) handleGetUpload(w http.ResponseWriter, r *http.Request, upload *database.Upload) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Получаем БД для этой выгрузки
	uploadDB, err := s.getUploadDatabase(upload.UploadUUID)
	if err != nil {
		s.writeJSONError(w, fmt.Sprintf("Failed to get upload database: %v", err), http.StatusInternalServerError)
		return
	}

	// Получаем детали выгрузки из БД upload
	_, catalogs, constants, err := uploadDB.GetUploadDetails(upload.UploadUUID)
	if err != nil {
		s.writeJSONError(w, fmt.Sprintf("Failed to get upload details: %v", err), http.StatusInternalServerError)
		return
	}

	// Получаем количество элементов для каждого справочника
	itemCounts, err := uploadDB.GetCatalogItemCountByCatalog(upload.ID)
	if err != nil {
		s.writeJSONError(w, fmt.Sprintf("Failed to get catalog item counts: %v", err), http.StatusInternalServerError)
		return
	}

	catalogInfos := make([]CatalogInfo, len(catalogs))
	for i, catalog := range catalogs {
		catalogInfos[i] = CatalogInfo{
			ID:        catalog.ID,
			Name:      catalog.Name,
			Synonym:   catalog.Synonym,
			ItemCount: itemCounts[catalog.ID],
			CreatedAt: catalog.CreatedAt,
		}
	}

	// Преобразуем константы в интерфейсы
	constantData := make([]interface{}, len(constants))
	for i, constant := range constants {
		constantData[i] = map[string]interface{}{
			"id":         constant.ID,
			"name":       constant.Name,
			"synonym":    constant.Synonym,
			"type":       constant.Type,
			"value":      constant.Value,
			"created_at": constant.CreatedAt,
		}
	}

	details := UploadDetails{
		UploadUUID:     upload.UploadUUID,
		StartedAt:      upload.StartedAt,
		CompletedAt:    upload.CompletedAt,
		Status:         upload.Status,
		Version1C:      upload.Version1C,
		ConfigName:     upload.ConfigName,
		TotalConstants: upload.TotalConstants,
		TotalCatalogs:  upload.TotalCatalogs,
		TotalItems:     upload.TotalItems,
		Catalogs:       catalogInfos,
		Constants:      constantData,
	}

	s.log(LogEntry{
		Timestamp:  time.Now(),
		Level:      "INFO",
		Message:    fmt.Sprintf("Upload details requested for %s", upload.UploadUUID),
		UploadUUID: upload.UploadUUID,
		Endpoint:   "/api/uploads/{uuid}",
	})

	s.writeJSONResponse(w, details, http.StatusOK)
}

// handleGetUploadData обрабатывает запрос данных выгрузки с фильтрацией и пагинацией
func (s *Server) handleGetUploadData(w http.ResponseWriter, r *http.Request, upload *database.Upload) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Парсим query параметры
	dataType := r.URL.Query().Get("type")
	if dataType == "" {
		dataType = "all"
	}

	catalogNamesStr := r.URL.Query().Get("catalog_names")
	var catalogNames []string
	if catalogNamesStr != "" {
		catalogNames = strings.Split(catalogNamesStr, ",")
		for i := range catalogNames {
			catalogNames[i] = strings.TrimSpace(catalogNames[i])
		}
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}

	offset := (page - 1) * limit

	var responseItems []DataItem
	var total int

	// Функция для экранирования XML
	escapeXML := func(s string) string {
		s = strings.ReplaceAll(s, "&", "&amp;")
		s = strings.ReplaceAll(s, "<", "&lt;")
		s = strings.ReplaceAll(s, ">", "&gt;")
		s = strings.ReplaceAll(s, "\"", "&quot;")
		s = strings.ReplaceAll(s, "'", "&apos;")
		return s
	}

	// Получаем БД для этой выгрузки
	uploadDB, err := s.getUploadDatabase(upload.UploadUUID)
	if err != nil {
		s.writeJSONError(w, fmt.Sprintf("Failed to get upload database: %v", err), http.StatusInternalServerError)
		return
	}

	// Получаем данные в зависимости от типа
	if dataType == "constants" {
		constants, err := uploadDB.GetConstantsByUpload(upload.ID)
		if err != nil {
			s.writeJSONError(w, fmt.Sprintf("Failed to get constants: %v", err), http.StatusInternalServerError)
			return
		}

		total = len(constants)

		// Применяем пагинацию для констант
		start := offset
		end := offset + limit
		if start > len(constants) {
			start = len(constants)
		}
		if end > len(constants) {
			end = len(constants)
		}

		for i := start; i < end; i++ {
			constData := constants[i]
			// Формируем XML для константы - включаем все поля из БД
			dataXML := fmt.Sprintf(`<constant><id>%d</id><upload_id>%d</upload_id><name>%s</name><synonym>%s</synonym><type>%s</type><value>%s</value><created_at>%s</created_at></constant>`,
				constData.ID, constData.UploadID, escapeXML(constData.Name), escapeXML(constData.Synonym),
				escapeXML(constData.Type), escapeXML(constData.Value), constData.CreatedAt.Format(time.RFC3339))

			responseItems = append(responseItems, DataItem{
				Type:      "constant",
				ID:        constData.ID,
				Data:      dataXML,
				CreatedAt: constData.CreatedAt,
			})
		}
	} else if dataType == "catalogs" {
		catalogItems, itemTotal, err := uploadDB.GetCatalogItemsByUpload(upload.ID, catalogNames, offset, limit)
		if err != nil {
			s.writeJSONError(w, fmt.Sprintf("Failed to get catalog items: %v", err), http.StatusInternalServerError)
			return
		}

		total = itemTotal

		for _, itemData := range catalogItems {
			// Формируем XML для элемента справочника
			// Включаем все поля из БД: id, catalog_id, catalog_name, reference, code, name, attributes_xml, table_parts_xml, created_at
			// attributes_xml и table_parts_xml уже содержат XML, вставляем их как есть (innerXML)
			dataXML := fmt.Sprintf(`<catalog_item><id>%d</id><catalog_id>%d</catalog_id><catalog_name>%s</catalog_name><reference>%s</reference><code>%s</code><name>%s</name><attributes_xml>%s</attributes_xml><table_parts_xml>%s</table_parts_xml><created_at>%s</created_at></catalog_item>`,
				itemData.ID, itemData.CatalogID, escapeXML(itemData.CatalogName),
				escapeXML(itemData.Reference), escapeXML(itemData.Code), escapeXML(itemData.Name),
				itemData.Attributes, itemData.TableParts, itemData.CreatedAt.Format(time.RFC3339))

			responseItems = append(responseItems, DataItem{
				Type:      "catalog_item",
				ID:        itemData.ID,
				Data:      dataXML,
				CreatedAt: itemData.CreatedAt,
			})
		}
	} else { // dataType == "all"
		// Для "all" сначала получаем все константы и элементы
		constants, err := s.db.GetConstantsByUpload(upload.ID)
		if err != nil {
			s.writeJSONError(w, fmt.Sprintf("Failed to get constants: %v", err), http.StatusInternalServerError)
			return
		}

		catalogItems, itemTotal, err := s.db.GetCatalogItemsByUpload(upload.ID, catalogNames, 0, 0)
		if err != nil {
			s.writeJSONError(w, fmt.Sprintf("Failed to get catalog items: %v", err), http.StatusInternalServerError)
			return
		}

		total = len(constants) + itemTotal

		// Объединяем все элементы и применяем пагинацию
		allItems := make([]DataItem, 0, total)

		// Добавляем константы - включаем все поля из БД
		for _, constant := range constants {
			dataXML := fmt.Sprintf(`<constant><id>%d</id><upload_id>%d</upload_id><name>%s</name><synonym>%s</synonym><type>%s</type><value>%s</value><created_at>%s</created_at></constant>`,
				constant.ID, constant.UploadID, escapeXML(constant.Name), escapeXML(constant.Synonym),
				escapeXML(constant.Type), escapeXML(constant.Value), constant.CreatedAt.Format(time.RFC3339))

			allItems = append(allItems, DataItem{
				Type:      "constant",
				ID:        constant.ID,
				Data:      dataXML,
				CreatedAt: constant.CreatedAt,
			})
		}

		// Добавляем элементы справочников
		for _, itemData := range catalogItems {
			// Включаем все поля из БД: id, catalog_id, catalog_name, reference, code, name, attributes_xml, table_parts_xml, created_at
			// attributes_xml и table_parts_xml уже содержат XML, вставляем их как есть (innerXML)
			dataXML := fmt.Sprintf(`<catalog_item><id>%d</id><catalog_id>%d</catalog_id><catalog_name>%s</catalog_name><reference>%s</reference><code>%s</code><name>%s</name><attributes_xml>%s</attributes_xml><table_parts_xml>%s</table_parts_xml><created_at>%s</created_at></catalog_item>`,
				itemData.ID, itemData.CatalogID, escapeXML(itemData.CatalogName),
				escapeXML(itemData.Reference), escapeXML(itemData.Code), escapeXML(itemData.Name),
				itemData.Attributes, itemData.TableParts, itemData.CreatedAt.Format(time.RFC3339))

			allItems = append(allItems, DataItem{
				Type:      "catalog_item",
				ID:        itemData.ID,
				Data:      dataXML,
				CreatedAt: itemData.CreatedAt,
			})
		}

		// Применяем пагинацию
		start := offset
		end := offset + limit
		if start > len(allItems) {
			start = len(allItems)
		}
		if end > len(allItems) {
			end = len(allItems)
		}

		responseItems = allItems[start:end]
	}

	// Формируем XML ответ
	response := DataResponse{
		UploadUUID: upload.UploadUUID,
		Type:       dataType,
		Page:       page,
		Limit:      limit,
		Total:      total,
		Items:      responseItems,
	}

	s.log(LogEntry{
		Timestamp:  time.Now(),
		Level:      "INFO",
		Message:    fmt.Sprintf("Upload data requested for %s, type=%s, returned %d items", upload.UploadUUID, dataType, len(responseItems)),
		UploadUUID: upload.UploadUUID,
		Endpoint:   "/api/uploads/{uuid}/data",
	})

	s.writeXMLResponse(w, response)
}

// handleStreamUploadData обрабатывает потоковую отправку данных через SSE
func (s *Server) handleStreamUploadData(w http.ResponseWriter, r *http.Request, upload *database.Upload) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Парсим query параметры
	dataType := r.URL.Query().Get("type")
	if dataType == "" {
		dataType = "all"
	}

	catalogNamesStr := r.URL.Query().Get("catalog_names")
	var catalogNames []string
	if catalogNamesStr != "" {
		catalogNames = strings.Split(catalogNamesStr, ",")
		for i := range catalogNames {
			catalogNames[i] = strings.TrimSpace(catalogNames[i])
		}
	}

	// Устанавливаем заголовки для SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		s.writeJSONError(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	s.log(LogEntry{
		Timestamp:  time.Now(),
		Level:      "INFO",
		Message:    fmt.Sprintf("Stream started for upload %s, type=%s", upload.UploadUUID, dataType),
		UploadUUID: upload.UploadUUID,
		Endpoint:   "/api/uploads/{uuid}/stream",
	})

	// Функция для экранирования XML
	escapeXML := func(s string) string {
		s = strings.ReplaceAll(s, "&", "&amp;")
		s = strings.ReplaceAll(s, "<", "&lt;")
		s = strings.ReplaceAll(s, ">", "&gt;")
		s = strings.ReplaceAll(s, "\"", "&quot;")
		s = strings.ReplaceAll(s, "'", "&apos;")
		return s
	}

	// Получаем БД для этой выгрузки
	uploadDB, err := s.getUploadDatabase(upload.UploadUUID)
	if err != nil {
		fmt.Fprintf(w, "data: {\"type\":\"error\",\"message\":\"Failed to get upload database: %v\"}\n\n", err)
		flusher.Flush()
		return
	}

	// Отправляем константы
	if dataType == "constants" || dataType == "all" {
		constants, err := uploadDB.GetConstantsByUpload(upload.ID)
		if err == nil {
			for _, constant := range constants {
				// Формируем XML для константы - включаем все поля из БД
				dataXML := fmt.Sprintf(`<constant><id>%d</id><upload_id>%d</upload_id><name>%s</name><synonym>%s</synonym><type>%s</type><value>%s</value><created_at>%s</created_at></constant>`,
					constant.ID, constant.UploadID, escapeXML(constant.Name), escapeXML(constant.Synonym),
					escapeXML(constant.Type), escapeXML(constant.Value), constant.CreatedAt.Format(time.RFC3339))

				item := DataItem{
					Type:      "constant",
					ID:        constant.ID,
					Data:      dataXML,
					CreatedAt: constant.CreatedAt,
				}

				// Отправляем как XML
				xmlData, _ := xml.Marshal(item)
				fmt.Fprintf(w, "data: %s\n\n", string(xmlData))
				flusher.Flush()
			}
		}
	}

	// Отправляем элементы справочников
	if dataType == "catalogs" || dataType == "all" {
		offset := 0
		limit := 100

		for {
			items, _, err := uploadDB.GetCatalogItemsByUpload(upload.ID, catalogNames, offset, limit)
			if err != nil || len(items) == 0 {
				break
			}

			for _, itemData := range items {
				// Формируем XML для элемента справочника
				// Включаем все поля из БД: id, catalog_id, catalog_name, reference, code, name, attributes_xml, table_parts_xml, created_at
				// attributes_xml и table_parts_xml уже содержат XML, вставляем их как есть (innerXML)
				dataXML := fmt.Sprintf(`<catalog_item><id>%d</id><catalog_id>%d</catalog_id><catalog_name>%s</catalog_name><reference>%s</reference><code>%s</code><name>%s</name><attributes_xml>%s</attributes_xml><table_parts_xml>%s</table_parts_xml><created_at>%s</created_at></catalog_item>`,
					itemData.ID, itemData.CatalogID, escapeXML(itemData.CatalogName),
					escapeXML(itemData.Reference), escapeXML(itemData.Code), escapeXML(itemData.Name),
					itemData.Attributes, itemData.TableParts, itemData.CreatedAt.Format(time.RFC3339))

				dataItem := DataItem{
					Type:      "catalog_item",
					ID:        itemData.ID,
					Data:      dataXML,
					CreatedAt: itemData.CreatedAt,
				}

				// Отправляем как XML
				xmlData, _ := xml.Marshal(dataItem)
				fmt.Fprintf(w, "data: %s\n\n", string(xmlData))
				flusher.Flush()
			}

			if len(items) < limit {
				break
			}

			offset += limit
		}
	}

	// Отправляем завершающее сообщение
	fmt.Fprintf(w, "data: {\"type\":\"complete\"}\n\n")
	flusher.Flush()
}

// handleVerifyUpload обрабатывает проверку успешной передачи
func (s *Server) handleVerifyUpload(w http.ResponseWriter, r *http.Request, upload *database.Upload) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req VerifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeJSONError(w, "Failed to parse request body", http.StatusBadRequest)
		return
	}

	// Получаем все ID элементов выгрузки
	receivedSet := make(map[int]bool)
	for _, id := range req.ReceivedIDs {
		receivedSet[id] = true
	}

	// Получаем БД для этой выгрузки
	uploadDB, err := s.getUploadDatabase(upload.UploadUUID)
	if err != nil {
		s.writeJSONError(w, fmt.Sprintf("Failed to get upload database: %v", err), http.StatusInternalServerError)
		return
	}

	// Получаем все константы
	constants, err := uploadDB.GetConstantsByUpload(upload.ID)
	if err != nil {
		s.writeJSONError(w, fmt.Sprintf("Failed to get constants: %v", err), http.StatusInternalServerError)
		return
	}

	// Получаем все элементы справочников
	catalogItems, _, err := uploadDB.GetCatalogItemsByUpload(upload.ID, nil, 0, 0)
	if err != nil {
		s.writeJSONError(w, fmt.Sprintf("Failed to get catalog items: %v", err), http.StatusInternalServerError)
		return
	}

	// Собираем все ожидаемые ID
	expectedSet := make(map[int]bool)
	for _, constant := range constants {
		expectedSet[constant.ID] = true
	}
	for _, item := range catalogItems {
		expectedSet[item.ID] = true
	}

	// Находим отсутствующие ID
	var missingIDs []int
	for id := range expectedSet {
		if !receivedSet[id] {
			missingIDs = append(missingIDs, id)
		}
	}

	expectedTotal := len(expectedSet)
	receivedCount := len(req.ReceivedIDs)
	isComplete := len(missingIDs) == 0

	message := fmt.Sprintf("Received %d of %d items", receivedCount, expectedTotal)
	if !isComplete {
		message += fmt.Sprintf(", %d items missing", len(missingIDs))
	} else {
		message += ", all items received"
	}

	response := VerifyResponse{
		UploadUUID:    upload.UploadUUID,
		ExpectedTotal: expectedTotal,
		ReceivedCount: receivedCount,
		MissingIDs:    missingIDs,
		IsComplete:    isComplete,
		Message:       message,
	}

	s.log(LogEntry{
		Timestamp:  time.Now(),
		Level:      "INFO",
		Message:    fmt.Sprintf("Verify requested for upload %s: %s", upload.UploadUUID, message),
		UploadUUID: upload.UploadUUID,
		Endpoint:   "/api/uploads/{uuid}/verify",
	})

	s.writeJSONResponse(w, response, http.StatusOK)
}

// handleGetUploadNormalized обрабатывает запрос детальной информации о выгрузке из нормализованной БД
func (s *Server) handleGetUploadNormalized(w http.ResponseWriter, r *http.Request, upload *database.Upload) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Получаем детали выгрузки из нормализованной БД
	_, catalogs, constants, err := s.normalizedDB.GetUploadDetails(upload.UploadUUID)
	if err != nil {
		s.writeJSONError(w, fmt.Sprintf("Failed to get normalized upload details: %v", err), http.StatusInternalServerError)
		return
	}

	// Получаем количество элементов для каждого справочника
	itemCounts, err := s.normalizedDB.GetCatalogItemCountByCatalog(upload.ID)
	if err != nil {
		s.writeJSONError(w, fmt.Sprintf("Failed to get catalog item counts: %v", err), http.StatusInternalServerError)
		return
	}

	catalogInfos := make([]CatalogInfo, len(catalogs))
	for i, catalog := range catalogs {
		catalogInfos[i] = CatalogInfo{
			ID:        catalog.ID,
			Name:      catalog.Name,
			Synonym:   catalog.Synonym,
			ItemCount: itemCounts[catalog.ID],
			CreatedAt: catalog.CreatedAt,
		}
	}

	// Преобразуем константы в интерфейсы
	constantData := make([]interface{}, len(constants))
	for i, constant := range constants {
		constantData[i] = map[string]interface{}{
			"id":         constant.ID,
			"name":       constant.Name,
			"synonym":    constant.Synonym,
			"type":       constant.Type,
			"value":      constant.Value,
			"created_at": constant.CreatedAt,
		}
	}

	details := UploadDetails{
		UploadUUID:     upload.UploadUUID,
		StartedAt:      upload.StartedAt,
		CompletedAt:    upload.CompletedAt,
		Status:         upload.Status,
		Version1C:      upload.Version1C,
		ConfigName:     upload.ConfigName,
		TotalConstants: upload.TotalConstants,
		TotalCatalogs:  upload.TotalCatalogs,
		TotalItems:     upload.TotalItems,
		Catalogs:       catalogInfos,
		Constants:      constantData,
	}

	s.log(LogEntry{
		Timestamp:  time.Now(),
		Level:      "INFO",
		Message:    fmt.Sprintf("Normalized upload details requested for %s", upload.UploadUUID),
		UploadUUID: upload.UploadUUID,
		Endpoint:   "/api/normalized/uploads/{uuid}",
	})

	s.writeJSONResponse(w, details, http.StatusOK)
}

// handleGetUploadDataNormalized обрабатывает запрос данных выгрузки из нормализованной БД
func (s *Server) handleGetUploadDataNormalized(w http.ResponseWriter, r *http.Request, upload *database.Upload) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Парсим query параметры
	dataType := r.URL.Query().Get("type")
	if dataType == "" {
		dataType = "all"
	}

	catalogNamesStr := r.URL.Query().Get("catalog_names")
	var catalogNames []string
	if catalogNamesStr != "" {
		catalogNames = strings.Split(catalogNamesStr, ",")
		for i := range catalogNames {
			catalogNames[i] = strings.TrimSpace(catalogNames[i])
		}
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}

	offset := (page - 1) * limit

	var responseItems []DataItem
	var total int

	// Функция для экранирования XML
	escapeXML := func(s string) string {
		s = strings.ReplaceAll(s, "&", "&amp;")
		s = strings.ReplaceAll(s, "<", "&lt;")
		s = strings.ReplaceAll(s, ">", "&gt;")
		s = strings.ReplaceAll(s, "\"", "&quot;")
		s = strings.ReplaceAll(s, "'", "&apos;")
		return s
	}

	// Получаем данные в зависимости от типа из нормализованной БД
	if dataType == "constants" {
		constants, err := s.normalizedDB.GetConstantsByUpload(upload.ID)
		if err != nil {
			s.writeJSONError(w, fmt.Sprintf("Failed to get constants: %v", err), http.StatusInternalServerError)
			return
		}

		total = len(constants)

		// Применяем пагинацию для констант
		start := offset
		end := offset + limit
		if start > len(constants) {
			start = len(constants)
		}
		if end > len(constants) {
			end = len(constants)
		}

		for i := start; i < end; i++ {
			constData := constants[i]
			// Формируем XML для константы - включаем все поля из БД
			dataXML := fmt.Sprintf(`<constant><id>%d</id><upload_id>%d</upload_id><name>%s</name><synonym>%s</synonym><type>%s</type><value>%s</value><created_at>%s</created_at></constant>`,
				constData.ID, constData.UploadID, escapeXML(constData.Name), escapeXML(constData.Synonym),
				escapeXML(constData.Type), escapeXML(constData.Value), constData.CreatedAt.Format(time.RFC3339))

			responseItems = append(responseItems, DataItem{
				Type:      "constant",
				ID:        constData.ID,
				Data:      dataXML,
				CreatedAt: constData.CreatedAt,
			})
		}
	} else if dataType == "catalogs" {
		catalogItems, itemTotal, err := s.normalizedDB.GetCatalogItemsByUpload(upload.ID, catalogNames, offset, limit)
		if err != nil {
			s.writeJSONError(w, fmt.Sprintf("Failed to get catalog items: %v", err), http.StatusInternalServerError)
			return
		}

		total = itemTotal

		for _, itemData := range catalogItems {
			// Формируем XML для элемента справочника
			// Включаем все поля из БД: id, catalog_id, catalog_name, reference, code, name, attributes_xml, table_parts_xml, created_at
			// attributes_xml и table_parts_xml уже содержат XML, вставляем их как есть (innerXML)
			dataXML := fmt.Sprintf(`<catalog_item><id>%d</id><catalog_id>%d</catalog_id><catalog_name>%s</catalog_name><reference>%s</reference><code>%s</code><name>%s</name><attributes_xml>%s</attributes_xml><table_parts_xml>%s</table_parts_xml><created_at>%s</created_at></catalog_item>`,
				itemData.ID, itemData.CatalogID, escapeXML(itemData.CatalogName),
				escapeXML(itemData.Reference), escapeXML(itemData.Code), escapeXML(itemData.Name),
				itemData.Attributes, itemData.TableParts, itemData.CreatedAt.Format(time.RFC3339))

			responseItems = append(responseItems, DataItem{
				Type:      "catalog_item",
				ID:        itemData.ID,
				Data:      dataXML,
				CreatedAt: itemData.CreatedAt,
			})
		}
	} else { // dataType == "all"
		// Для "all" сначала получаем все константы и элементы
		constants, err := s.normalizedDB.GetConstantsByUpload(upload.ID)
		if err != nil {
			s.writeJSONError(w, fmt.Sprintf("Failed to get constants: %v", err), http.StatusInternalServerError)
			return
		}

		catalogItems, itemTotal, err := s.normalizedDB.GetCatalogItemsByUpload(upload.ID, catalogNames, 0, 0)
		if err != nil {
			s.writeJSONError(w, fmt.Sprintf("Failed to get catalog items: %v", err), http.StatusInternalServerError)
			return
		}

		total = len(constants) + itemTotal

		// Объединяем все элементы и применяем пагинацию
		allItems := make([]DataItem, 0, total)

		// Добавляем константы - включаем все поля из БД
		for _, constant := range constants {
			dataXML := fmt.Sprintf(`<constant><id>%d</id><upload_id>%d</upload_id><name>%s</name><synonym>%s</synonym><type>%s</type><value>%s</value><created_at>%s</created_at></constant>`,
				constant.ID, constant.UploadID, escapeXML(constant.Name), escapeXML(constant.Synonym),
				escapeXML(constant.Type), escapeXML(constant.Value), constant.CreatedAt.Format(time.RFC3339))

			allItems = append(allItems, DataItem{
				Type:      "constant",
				ID:        constant.ID,
				Data:      dataXML,
				CreatedAt: constant.CreatedAt,
			})
		}

		// Добавляем элементы справочников
		for _, itemData := range catalogItems {
			// Включаем все поля из БД: id, catalog_id, catalog_name, reference, code, name, attributes_xml, table_parts_xml, created_at
			// attributes_xml и table_parts_xml уже содержат XML, вставляем их как есть (innerXML)
			dataXML := fmt.Sprintf(`<catalog_item><id>%d</id><catalog_id>%d</catalog_id><catalog_name>%s</catalog_name><reference>%s</reference><code>%s</code><name>%s</name><attributes_xml>%s</attributes_xml><table_parts_xml>%s</table_parts_xml><created_at>%s</created_at></catalog_item>`,
				itemData.ID, itemData.CatalogID, escapeXML(itemData.CatalogName),
				escapeXML(itemData.Reference), escapeXML(itemData.Code), escapeXML(itemData.Name),
				itemData.Attributes, itemData.TableParts, itemData.CreatedAt.Format(time.RFC3339))

			allItems = append(allItems, DataItem{
				Type:      "catalog_item",
				ID:        itemData.ID,
				Data:      dataXML,
				CreatedAt: itemData.CreatedAt,
			})
		}

		// Применяем пагинацию
		start := offset
		end := offset + limit
		if start > len(allItems) {
			start = len(allItems)
		}
		if end > len(allItems) {
			end = len(allItems)
		}

		responseItems = allItems[start:end]
	}

	// Формируем XML ответ
	response := DataResponse{
		UploadUUID: upload.UploadUUID,
		Type:       dataType,
		Page:       page,
		Limit:      limit,
		Total:      total,
		Items:      responseItems,
	}

	s.log(LogEntry{
		Timestamp:  time.Now(),
		Level:      "INFO",
		Message:    fmt.Sprintf("Normalized upload data requested for %s, type=%s, returned %d items", upload.UploadUUID, dataType, len(responseItems)),
		UploadUUID: upload.UploadUUID,
		Endpoint:   "/api/normalized/uploads/{uuid}/data",
	})

	s.writeXMLResponse(w, response)
}

// handleStreamUploadDataNormalized обрабатывает потоковую отправку данных из нормализованной БД через SSE
func (s *Server) handleStreamUploadDataNormalized(w http.ResponseWriter, r *http.Request, upload *database.Upload) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Парсим query параметры
	dataType := r.URL.Query().Get("type")
	if dataType == "" {
		dataType = "all"
	}

	catalogNamesStr := r.URL.Query().Get("catalog_names")
	var catalogNames []string
	if catalogNamesStr != "" {
		catalogNames = strings.Split(catalogNamesStr, ",")
		for i := range catalogNames {
			catalogNames[i] = strings.TrimSpace(catalogNames[i])
		}
	}

	// Устанавливаем заголовки для SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	flusher, ok := w.(http.Flusher)
	if !ok {
		s.writeJSONError(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	s.log(LogEntry{
		Timestamp:  time.Now(),
		Level:      "INFO",
		Message:    fmt.Sprintf("Normalized stream started for upload %s, type=%s", upload.UploadUUID, dataType),
		UploadUUID: upload.UploadUUID,
		Endpoint:   "/api/normalized/uploads/{uuid}/stream",
	})

	// Функция для экранирования XML
	escapeXML := func(s string) string {
		s = strings.ReplaceAll(s, "&", "&amp;")
		s = strings.ReplaceAll(s, "<", "&lt;")
		s = strings.ReplaceAll(s, ">", "&gt;")
		s = strings.ReplaceAll(s, "\"", "&quot;")
		s = strings.ReplaceAll(s, "'", "&apos;")
		return s
	}

	// Отправляем константы из нормализованной БД
	if dataType == "constants" || dataType == "all" {
		constants, err := s.normalizedDB.GetConstantsByUpload(upload.ID)
		if err == nil {
			for _, constant := range constants {
				// Формируем XML для константы - включаем все поля из БД
				dataXML := fmt.Sprintf(`<constant><id>%d</id><upload_id>%d</upload_id><name>%s</name><synonym>%s</synonym><type>%s</type><value>%s</value><created_at>%s</created_at></constant>`,
					constant.ID, constant.UploadID, escapeXML(constant.Name), escapeXML(constant.Synonym),
					escapeXML(constant.Type), escapeXML(constant.Value), constant.CreatedAt.Format(time.RFC3339))

				item := DataItem{
					Type:      "constant",
					ID:        constant.ID,
					Data:      dataXML,
					CreatedAt: constant.CreatedAt,
				}

				// Отправляем как XML
				xmlData, _ := xml.Marshal(item)
				fmt.Fprintf(w, "data: %s\n\n", string(xmlData))
				flusher.Flush()
			}
		}
	}

	// Отправляем элементы справочников из нормализованной БД
	if dataType == "catalogs" || dataType == "all" {
		offset := 0
		limit := 100

		for {
			items, _, err := s.normalizedDB.GetCatalogItemsByUpload(upload.ID, catalogNames, offset, limit)
			if err != nil || len(items) == 0 {
				break
			}

			for _, itemData := range items {
				// Формируем XML для элемента справочника
				// Включаем все поля из БД: id, catalog_id, catalog_name, reference, code, name, attributes_xml, table_parts_xml, created_at
				// attributes_xml и table_parts_xml уже содержат XML, вставляем их как есть (innerXML)
				dataXML := fmt.Sprintf(`<catalog_item><id>%d</id><catalog_id>%d</catalog_id><catalog_name>%s</catalog_name><reference>%s</reference><code>%s</code><name>%s</name><attributes_xml>%s</attributes_xml><table_parts_xml>%s</table_parts_xml><created_at>%s</created_at></catalog_item>`,
					itemData.ID, itemData.CatalogID, escapeXML(itemData.CatalogName),
					escapeXML(itemData.Reference), escapeXML(itemData.Code), escapeXML(itemData.Name),
					itemData.Attributes, itemData.TableParts, itemData.CreatedAt.Format(time.RFC3339))

				dataItem := DataItem{
					Type:      "catalog_item",
					ID:        itemData.ID,
					Data:      dataXML,
					CreatedAt: itemData.CreatedAt,
				}

				// Отправляем как XML
				xmlData, _ := xml.Marshal(dataItem)
				fmt.Fprintf(w, "data: %s\n\n", string(xmlData))
				flusher.Flush()
			}

			if len(items) < limit {
				break
			}

			offset += limit
		}
	}

	// Отправляем завершающее сообщение
	fmt.Fprintf(w, "data: {\"type\":\"complete\"}\n\n")
	flusher.Flush()
}

// handleVerifyUploadNormalized обрабатывает проверку успешной передачи для нормализованной БД
func (s *Server) handleVerifyUploadNormalized(w http.ResponseWriter, r *http.Request, upload *database.Upload) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req VerifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeJSONError(w, "Failed to parse request body", http.StatusBadRequest)
		return
	}

	// Получаем все ID элементов выгрузки из нормализованной БД
	receivedSet := make(map[int]bool)
	for _, id := range req.ReceivedIDs {
		receivedSet[id] = true
	}

	// Получаем все константы
	constants, err := s.normalizedDB.GetConstantsByUpload(upload.ID)
	if err != nil {
		s.writeJSONError(w, fmt.Sprintf("Failed to get constants: %v", err), http.StatusInternalServerError)
		return
	}

	// Получаем все элементы справочников
	catalogItems, _, err := s.normalizedDB.GetCatalogItemsByUpload(upload.ID, nil, 0, 0)
	if err != nil {
		s.writeJSONError(w, fmt.Sprintf("Failed to get catalog items: %v", err), http.StatusInternalServerError)
		return
	}

	// Собираем все ожидаемые ID
	expectedSet := make(map[int]bool)
	for _, constant := range constants {
		expectedSet[constant.ID] = true
	}
	for _, item := range catalogItems {
		expectedSet[item.ID] = true
	}

	// Находим отсутствующие ID
	var missingIDs []int
	for id := range expectedSet {
		if !receivedSet[id] {
			missingIDs = append(missingIDs, id)
		}
	}

	expectedTotal := len(expectedSet)
	receivedCount := len(req.ReceivedIDs)
	isComplete := len(missingIDs) == 0

	message := fmt.Sprintf("Received %d of %d items", receivedCount, expectedTotal)
	if !isComplete {
		message += fmt.Sprintf(", %d items missing", len(missingIDs))
	} else {
		message += ", all items received"
	}

	response := VerifyResponse{
		UploadUUID:    upload.UploadUUID,
		ExpectedTotal: expectedTotal,
		ReceivedCount: receivedCount,
		MissingIDs:    missingIDs,
		IsComplete:    isComplete,
		Message:       message,
	}

	s.log(LogEntry{
		Timestamp:  time.Now(),
		Level:      "INFO",
		Message:    fmt.Sprintf("Normalized verify requested for upload %s: %s", upload.UploadUUID, message),
		UploadUUID: upload.UploadUUID,
		Endpoint:   "/api/normalized/uploads/{uuid}/verify",
	})

	s.writeJSONResponse(w, response, http.StatusOK)
}

// handleNormalizedHandshake обрабатывает рукопожатие для нормализованных данных
func (s *Server) handleNormalizedHandshake(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.writeErrorResponse(w, "Failed to read request body", err)
		return
	}

	var req HandshakeRequest
	if err := xml.Unmarshal(body, &req); err != nil {
		s.writeErrorResponse(w, "Failed to parse XML", err)
		return
	}

	// Создаем новую выгрузку в нормализованной БД
	uploadUUID := uuid.New().String()
	_, err = s.normalizedDB.CreateUpload(uploadUUID, req.Version1C, req.ConfigName)
	if err != nil {
		s.writeErrorResponse(w, "Failed to create normalized upload", err)
		return
	}

	s.log(LogEntry{
		Timestamp:  time.Now(),
		Level:      "INFO",
		Message:    fmt.Sprintf("Normalized handshake successful for upload %s", uploadUUID),
		UploadUUID: uploadUUID,
		Endpoint:   "/api/normalized/upload/handshake",
	})

	response := HandshakeResponse{
		Success:    true,
		UploadUUID: uploadUUID,
		Message:    "Normalized handshake successful",
		Timestamp:  time.Now().Format(time.RFC3339),
	}

	s.writeXMLResponse(w, response)
}

// handleNormalizedMetadata обрабатывает метаинформацию для нормализованных данных
func (s *Server) handleNormalizedMetadata(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.writeErrorResponse(w, "Failed to read request body", err)
		return
	}

	var req MetadataRequest
	if err := xml.Unmarshal(body, &req); err != nil {
		s.writeErrorResponse(w, "Failed to parse XML", err)
		return
	}

	// Проверяем существование выгрузки в нормализованной БД
	_, err = s.normalizedDB.GetUploadByUUID(req.UploadUUID)
	if err != nil {
		s.writeErrorResponse(w, "Normalized upload not found", err)
		return
	}

	s.log(LogEntry{
		Timestamp:  time.Now(),
		Level:      "INFO",
		Message:    "Normalized metadata received successfully",
		UploadUUID: req.UploadUUID,
		Endpoint:   "/api/normalized/upload/metadata",
	})

	response := MetadataResponse{
		Success:   true,
		Message:   "Normalized metadata received successfully",
		Timestamp: time.Now().Format(time.RFC3339),
	}

	s.writeXMLResponse(w, response)
}

// handleNormalizedConstant обрабатывает константу для нормализованных данных
func (s *Server) handleNormalizedConstant(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.writeErrorResponse(w, "Failed to read request body", err)
		return
	}

	var req ConstantRequest
	if err := xml.Unmarshal(body, &req); err != nil {
		s.writeErrorResponse(w, "Failed to parse XML", err)
		return
	}

	// Получаем выгрузку из нормализованной БД
	upload, err := s.normalizedDB.GetUploadByUUID(req.UploadUUID)
	if err != nil {
		s.writeErrorResponse(w, "Normalized upload not found", err)
		return
	}

	// Добавляем константу в нормализованную БД
	// req.Value теперь структура ConstantValue, используем Content для получения XML строки
	valueContent := req.Value.Content
	if err := s.normalizedDB.AddConstant(upload.ID, req.Name, req.Synonym, req.Type, valueContent); err != nil {
		s.writeErrorResponse(w, "Failed to add normalized constant", err)
		return
	}

	s.log(LogEntry{
		Timestamp:  time.Now(),
		Level:      "INFO",
		Message:    fmt.Sprintf("Normalized constant '%s' added successfully", req.Name),
		UploadUUID: req.UploadUUID,
		Endpoint:   "/api/normalized/upload/constant",
	})

	response := ConstantResponse{
		Success:   true,
		Message:   "Normalized constant added successfully",
		Timestamp: time.Now().Format(time.RFC3339),
	}

	s.writeXMLResponse(w, response)
}

// handleNormalizedCatalogMeta обрабатывает метаданные справочника для нормализованных данных
func (s *Server) handleNormalizedCatalogMeta(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.writeErrorResponse(w, "Failed to read request body", err)
		return
	}

	var req CatalogMetaRequest
	if err := xml.Unmarshal(body, &req); err != nil {
		s.writeErrorResponse(w, "Failed to parse XML", err)
		return
	}

	// Получаем выгрузку из нормализованной БД
	upload, err := s.normalizedDB.GetUploadByUUID(req.UploadUUID)
	if err != nil {
		s.writeErrorResponse(w, "Normalized upload not found", err)
		return
	}

	// Добавляем справочник в нормализованную БД
	catalog, err := s.normalizedDB.AddCatalog(upload.ID, req.Name, req.Synonym)
	if err != nil {
		s.writeErrorResponse(w, "Failed to add normalized catalog", err)
		return
	}

	s.log(LogEntry{
		Timestamp:  time.Now(),
		Level:      "INFO",
		Message:    fmt.Sprintf("Normalized catalog '%s' metadata added successfully", req.Name),
		UploadUUID: req.UploadUUID,
		Endpoint:   "/api/normalized/upload/catalog/meta",
	})

	response := CatalogMetaResponse{
		Success:   true,
		CatalogID: catalog.ID,
		Message:   "Normalized catalog metadata added successfully",
		Timestamp: time.Now().Format(time.RFC3339),
	}

	s.writeXMLResponse(w, response)
}

// handleNormalizedCatalogItem обрабатывает элемент справочника для нормализованных данных
func (s *Server) handleNormalizedCatalogItem(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.writeErrorResponse(w, "Failed to read request body", err)
		return
	}

	var req CatalogItemRequest
	if err := xml.Unmarshal(body, &req); err != nil {
		s.writeErrorResponse(w, "Failed to parse XML", err)
		return
	}

	// Получаем выгрузку из нормализованной БД
	upload, err := s.normalizedDB.GetUploadByUUID(req.UploadUUID)
	if err != nil {
		s.writeErrorResponse(w, "Normalized upload not found", err)
		return
	}

	// Находим справочник по имени в нормализованной БД
	var catalogID int
	err = s.normalizedDB.QueryRow("SELECT id FROM catalogs WHERE upload_id = ? AND name = ?", upload.ID, req.CatalogName).Scan(&catalogID)
	if err != nil {
		s.writeErrorResponse(w, "Normalized catalog not found", err)
		return
	}

	// Attributes и TableParts уже приходят как XML строки
	// Передаем их напрямую как строки
	if err := s.normalizedDB.AddCatalogItem(catalogID, req.Reference, req.Code, req.Name, req.Attributes, req.TableParts); err != nil {
		s.writeErrorResponse(w, "Failed to add normalized catalog item", err)
		return
	}

	s.log(LogEntry{
		Timestamp:  time.Now(),
		Level:      "INFO",
		Message:    fmt.Sprintf("Normalized catalog item '%s' added successfully", req.Name),
		UploadUUID: req.UploadUUID,
		Endpoint:   "/api/normalized/upload/catalog/item",
	})

	response := CatalogItemResponse{
		Success:   true,
		Message:   "Normalized catalog item added successfully",
		Timestamp: time.Now().Format(time.RFC3339),
	}

	s.writeXMLResponse(w, response)
}

// handleNormalizedComplete обрабатывает завершение выгрузки нормализованных данных
func (s *Server) handleNormalizedComplete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		s.writeErrorResponse(w, "Failed to read request body", err)
		return
	}

	var req CompleteRequest
	if err := xml.Unmarshal(body, &req); err != nil {
		s.writeErrorResponse(w, "Failed to parse XML", err)
		return
	}

	// Получаем выгрузку из нормализованной БД
	upload, err := s.normalizedDB.GetUploadByUUID(req.UploadUUID)
	if err != nil {
		s.writeErrorResponse(w, "Normalized upload not found", err)
		return
	}

	// Завершаем выгрузку в нормализованной БД
	if err := s.normalizedDB.CompleteUpload(upload.ID); err != nil {
		s.writeErrorResponse(w, "Failed to complete normalized upload", err)
		return
	}

	s.log(LogEntry{
		Timestamp:  time.Now(),
		Level:      "INFO",
		Message:    fmt.Sprintf("Normalized upload %s completed successfully", req.UploadUUID),
		UploadUUID: req.UploadUUID,
		Endpoint:   "/api/normalized/upload/complete",
	})

	response := CompleteResponse{
		Success:   true,
		Message:   "Normalized upload completed successfully",
		Timestamp: time.Now().Format(time.RFC3339),
	}

	s.writeXMLResponse(w, response)
}

// startNomenclatureProcessing запускает обработку номенклатуры
func (s *Server) startNomenclatureProcessing(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Получаем конфигурацию из менеджера воркеров, если доступен
	var config nomenclature.Config
	if s.workerConfigManager != nil {
		workerConfig, err := s.workerConfigManager.GetNomenclatureConfig()
		if err == nil {
			config = workerConfig
			config.DatabasePath = "./normalized_data.db"
		} else {
			// Fallback на дефолтную конфигурацию
			apiKey := os.Getenv("ARLIAI_API_KEY")
			if apiKey == "" {
				s.writeJSONError(w, "ARLIAI_API_KEY environment variable not set", http.StatusInternalServerError)
				return
			}
			config = nomenclature.DefaultConfig()
			config.ArliaiAPIKey = apiKey
			config.DatabasePath = "./normalized_data.db"
		}
	} else {
		// Fallback на дефолтную конфигурацию
		apiKey := os.Getenv("ARLIAI_API_KEY")
		if apiKey == "" {
			s.writeJSONError(w, "ARLIAI_API_KEY environment variable not set", http.StatusInternalServerError)
			return
		}
		config = nomenclature.DefaultConfig()
		config.ArliaiAPIKey = apiKey
		config.DatabasePath = "./normalized_data.db"
	}

	// Создаем процессор
	processor, err := nomenclature.NewProcessor(config)
	if err != nil {
		s.writeJSONError(w, fmt.Sprintf("Failed to create processor: %v", err), http.StatusInternalServerError)
		return
	}

	// Сохраняем процессор в сервере (заменяем старый, если есть)
	s.processorMutex.Lock()
	s.nomenclatureProcessor = processor
	s.processorMutex.Unlock()

	// Запускаем обработку в горутине
	go func() {
		defer func() {
			processor.Close()
			// Очищаем процессор после завершения (опционально, можно оставить для просмотра итогов)
			s.processorMutex.Lock()
			// Не очищаем сразу, оставляем для просмотра итогов до следующего запуска
			s.processorMutex.Unlock()
		}()
		if err := processor.ProcessAll(); err != nil {
			log.Printf("Ошибка обработки номенклатуры: %v", err)
		}
	}()

	s.log(LogEntry{
		Timestamp: time.Now(),
		Level:     "INFO",
		Message:   "Nomenclature processing started",
		Endpoint:  "/api/nomenclature/process",
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "processing_started",
		"message": "Обработка номенклатуры запущена",
	})
}

// getNomenclatureDBStats получает статистику из базы данных
func (s *Server) getNomenclatureDBStats(db *database.DB) (DBStatsResponse, error) {
	var stats DBStatsResponse

	// Общее количество записей
	row := db.QueryRow("SELECT COUNT(*) FROM catalog_items")
	err := row.Scan(&stats.Total)
	if err != nil {
		return stats, fmt.Errorf("failed to get total count: %v", err)
	}

	// Количество обработанных
	row = db.QueryRow("SELECT COUNT(*) FROM catalog_items WHERE processing_status = 'completed'")
	err = row.Scan(&stats.Completed)
	if err != nil {
		return stats, fmt.Errorf("failed to get completed count: %v", err)
	}

	// Количество с ошибками
	row = db.QueryRow("SELECT COUNT(*) FROM catalog_items WHERE processing_status = 'error'")
	err = row.Scan(&stats.Errors)
	if err != nil {
		return stats, fmt.Errorf("failed to get error count: %v", err)
	}

	// Количество ожидающих обработки
	row = db.QueryRow("SELECT COUNT(*) FROM catalog_items WHERE processing_status IS NULL OR processing_status = 'pending'")
	err = row.Scan(&stats.Pending)
	if err != nil {
		return stats, fmt.Errorf("failed to get pending count: %v", err)
	}

	return stats, nil
}

// handleNormalizeStart запускает процесс нормализации данных
func (s *Server) handleNormalizeStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Парсим тело запроса для получения AI конфигурации
	type NormalizeRequest struct {
		UseAI            bool    `json:"use_ai"`
		MinConfidence    float64 `json:"min_confidence"`
		RateLimitDelayMS int     `json:"rate_limit_delay_ms"`
		MaxRetries       int     `json:"max_retries"`
		Model            string  `json:"model"`     // Выбранная модель AI
		Database         string  `json:"database"`  // База данных для нормализации
		UseKpved         bool    `json:"use_kpved"` // Включить КПВЭД классификацию
	}

	var req NormalizeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Если тело пустое или некорректное, используем значения по умолчанию
		req.UseAI = false
		req.MinConfidence = 0.8
		req.RateLimitDelayMS = 100
		req.MaxRetries = 3
	}

	// Проверяем, не запущен ли уже процесс
	s.normalizerMutex.Lock()
	if s.normalizerRunning {
		s.normalizerMutex.Unlock()
		s.writeJSONError(w, "Normalization is already running", http.StatusConflict)
		return
	}
	s.normalizerRunning = true
	s.normalizerStartTime = time.Now()
	s.normalizerProcessed = 0
	s.normalizerSuccess = 0
	s.normalizerErrors = 0
	s.normalizerMutex.Unlock()

	// Загружаем конфигурацию нормализации из serviceDB
	config, err := s.serviceDB.GetNormalizationConfig()
	if err != nil {
		log.Printf("Ошибка получения конфигурации нормализации: %v, используем дефолтные значения", err)
		config = &database.NormalizationConfig{
			SourceTable:     "catalog_items",
			ReferenceColumn: "reference",
			CodeColumn:      "code",
			NameColumn:      "name",
		}
	}

	// Применяем конфигурацию к нормализатору
	s.normalizer.SetSourceConfig(
		config.SourceTable,
		config.ReferenceColumn,
		config.CodeColumn,
		config.NameColumn,
	)

	log.Printf("Запуск нормализации: таблица=%s, ref=%s, code=%s, name=%s",
		config.SourceTable, config.ReferenceColumn, config.CodeColumn, config.NameColumn)
	if req.UseAI {
		log.Printf("AI параметры из запроса игнорируются, используется конфигурация сервера")
	}

	// Если указана модель, устанавливаем её как активную через WorkerConfigManager
	if req.Model != "" && s.workerConfigManager != nil {
		// Валидируем имя модели
		if err := ValidateModelName(req.Model); err != nil {
			log.Printf("[normalize-start] Warning: Invalid model name %s: %v", req.Model, err)
			s.normalizerEvents <- fmt.Sprintf("⚠ Предупреждение: неверное имя модели %s: %v", req.Model, err)
			// Санитизируем и продолжаем
			req.Model = SanitizeModelName(req.Model)
		}

		provider, err := s.workerConfigManager.GetActiveProvider()
		if err == nil {
			// Устанавливаем модель как активную для провайдера
			if err := s.workerConfigManager.SetDefaultModel(provider.Name, req.Model); err != nil {
				log.Printf("[normalize-start] Warning: Failed to set model %s: %v", req.Model, err)
				s.normalizerEvents <- fmt.Sprintf("⚠ Предупреждение: не удалось установить модель %s: %v", req.Model, err)
			} else {
				log.Printf("[normalize-start] Установлена модель для нормализации: %s (провайдер: %s)", req.Model, provider.Name)
				s.normalizerEvents <- fmt.Sprintf("✓ Модель установлена: %s", req.Model)

				// Получаем информацию о модели для логирования
				model, modelErr := s.workerConfigManager.GetActiveModel(provider.Name)
				if modelErr == nil {
					log.Printf("[normalize-start] Параметры модели: скорость=%s, качество=%s, max_tokens=%d, temperature=%.2f",
						model.Speed, model.Quality, model.MaxTokens, model.Temperature)
				}
			}
		} else {
			log.Printf("[normalize-start] Warning: Failed to get active provider: %v", err)
		}
	}

	// Если указана база данных в запросе, открываем её и создаем новый normalizer
	var normalizerToUse *normalization.Normalizer
	var tempDB *database.DB

	if req.Database != "" {
		log.Printf("Открытие базы данных: %s", req.Database)
		s.normalizerEvents <- fmt.Sprintf("Открытие базы данных: %s", req.Database)

		var err error
		tempDB, err = database.NewDB(req.Database)
		if err != nil {
			s.normalizerMutex.Lock()
			s.normalizerRunning = false
			s.normalizerMutex.Unlock()
			s.writeJSONError(w, fmt.Sprintf("Failed to open database %s: %v", req.Database, err), http.StatusInternalServerError)
			return
		}

		// Создаем временный normalizer для указанной БД
		aiConfig := &normalization.AIConfig{
			Enabled:        true,
			MinConfidence:  0.7,
			RateLimitDelay: 100 * time.Millisecond,
			MaxRetries:     3,
		}
		// Создаем нормализатор
		normalizerToUse = normalization.NewNormalizer(tempDB, s.normalizerEvents, aiConfig)
		normalizerToUse.SetSourceConfig(
			config.SourceTable,
			config.ReferenceColumn,
			config.CodeColumn,
			config.NameColumn,
		)
		log.Printf("Создан временный normalizer для БД: %s (с WorkerConfigManager)", req.Database)
	} else {
		// Используем стандартный normalizer
		normalizerToUse = s.normalizer
		log.Printf("Используется стандартный normalizer")
	}

	// Запускаем нормализацию в горутине
	go func() {
		defer func() {
			// Закрываем временную БД если она была открыта
			if tempDB != nil {
				tempDB.Close()
				log.Printf("Временная БД %s закрыта", req.Database)
			}

			s.normalizerMutex.Lock()
			s.normalizerRunning = false
			s.normalizerMutex.Unlock()
			log.Println("Процесс нормализации завершен, флаг isRunning сброшен")
		}()

		log.Println("Запуск процесса нормализации в горутине...")
		s.normalizerEvents <- "Начало нормализации данных..."

		// Отслеживаем события для обновления статистики
		eventTicker := time.NewTicker(2 * time.Second)
		defer eventTicker.Stop()

		go func() {
			for range eventTicker.C {
				if !s.normalizerRunning {
					return
				}
				// Обновляем processed из БД
				var count int
				if err := s.db.QueryRow("SELECT COUNT(*) FROM normalized_data").Scan(&count); err == nil {
					s.normalizerMutex.Lock()
					s.normalizerProcessed = count
					s.normalizerMutex.Unlock()
				}
			}
		}()

		if err := normalizerToUse.ProcessNormalization(); err != nil {
			log.Printf("Ошибка нормализации данных: %v", err)
			s.normalizerEvents <- fmt.Sprintf("Ошибка нормализации: %v", err)
			s.normalizerMutex.Lock()
			s.normalizerErrors++
			s.normalizerMutex.Unlock()
		} else {
			log.Println("Нормализация завершена успешно")
			s.normalizerEvents <- "Нормализация завершена успешно"
			// Обновляем финальную статистику
			var finalCount int
			if err := s.db.QueryRow("SELECT COUNT(*) FROM normalized_data").Scan(&finalCount); err == nil {
				s.normalizerMutex.Lock()
				s.normalizerProcessed = finalCount
				s.normalizerSuccess = finalCount
				s.normalizerMutex.Unlock()
			}

			// КПВЭД классификация после нормализации
			if req.UseKpved && s.hierarchicalClassifier != nil {
				log.Println("Начинаем КПВЭД классификацию...")
				s.normalizerEvents <- "Начало КПВЭД классификации"

				s.kpvedClassifierMutex.RLock()
				classifier := s.hierarchicalClassifier
				s.kpvedClassifierMutex.RUnlock()

				if classifier != nil {
					// Определяем какую БД использовать: временную или стандартную
					dbToUse := s.normalizedDB
					if tempDB != nil {
						dbToUse = tempDB
					}

					// Получаем записи без КПВЭД классификации
					rows, err := dbToUse.Query(`
						SELECT id, normalized_name, category
						FROM normalized_data
						WHERE (kpved_code IS NULL OR kpved_code = '' OR TRIM(kpved_code) = '')
					`)
					if err != nil {
						log.Printf("Ошибка получения записей для КПВЭД классификации: %v", err)
						s.normalizerEvents <- fmt.Sprintf("Ошибка КПВЭД: %v", err)
					} else {
						defer rows.Close()

						var recordsToClassify []struct {
							ID             int
							NormalizedName string
							Category       string
						}

						for rows.Next() {
							var record struct {
								ID             int
								NormalizedName string
								Category       string
							}
							if err := rows.Scan(&record.ID, &record.NormalizedName, &record.Category); err != nil {
								log.Printf("Ошибка сканирования записи: %v", err)
								continue
							}
							recordsToClassify = append(recordsToClassify, record)
						}

						totalToClassify := len(recordsToClassify)
						if totalToClassify == 0 {
							log.Println("Нет записей для КПВЭД классификации")
							s.normalizerEvents <- "Все записи уже классифицированы по КПВЭД"
						} else {
							log.Printf("Найдено записей для КПВЭД классификации: %d", totalToClassify)
							s.normalizerEvents <- fmt.Sprintf("Классификация %d записей по КПВЭД", totalToClassify)

							classified := 0
							failed := 0
							for i, record := range recordsToClassify {
								// Классифицируем запись
								result, err := classifier.Classify(record.NormalizedName, record.Category)
								if err != nil {
									log.Printf("Ошибка классификации записи %d: %v", record.ID, err)
									failed++
									continue
								}

								// Обновляем запись с результатами классификации
								_, err = dbToUse.Exec(`
									UPDATE normalized_data
									SET kpved_code = ?, kpved_name = ?, kpved_confidence = ?
									WHERE id = ?
								`, result.FinalCode, result.FinalName, result.FinalConfidence, record.ID)

								if err != nil {
									log.Printf("Ошибка обновления КПВЭД для записи %d: %v", record.ID, err)
									failed++
									continue
								}

								classified++

								// Логируем прогресс каждые 10 записей или на последней записи
								if (i+1)%10 == 0 || i+1 == totalToClassify {
									progress := float64(i+1) / float64(totalToClassify) * 100
									log.Printf("КПВЭД классификация: %d/%d (%.1f%%)", i+1, totalToClassify, progress)
									s.normalizerEvents <- fmt.Sprintf("КПВЭД: %d/%d (%.1f%%)", i+1, totalToClassify, progress)
								}
							}

							log.Printf("КПВЭД классификация завершена: классифицировано %d из %d записей (ошибок: %d)", classified, totalToClassify, failed)
							s.normalizerEvents <- fmt.Sprintf("КПВЭД классификация завершена: %d/%d (ошибок: %d)", classified, totalToClassify, failed)
						}
					}
				} else {
					log.Println("КПВЭД классификатор недоступен")
					s.normalizerEvents <- "КПВЭД классификатор недоступен"
				}
			} else if req.UseKpved {
				log.Println("КПВЭД классификация запрошена, но классификатор не инициализирован")
				s.normalizerEvents <- "КПВЭД классификатор не инициализирован. Проверьте ARLIAI_API_KEY"
			}
		}
	}()

	s.log(LogEntry{
		Timestamp: time.Now(),
		Level:     "INFO",
		Message:   "Нормализация данных запущена",
		Endpoint:  "/api/normalize/start",
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":   true,
		"message":   "Нормализация данных запущена",
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// handleNormalizationEvents обрабатывает SSE соединение для событий нормализации
func (s *Server) handleNormalizationEvents(w http.ResponseWriter, r *http.Request) {
	// Устанавливаем заголовки для SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Cache-Control")

	// Проверяем поддержку Flusher
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// Отправляем начальное событие
	fmt.Fprintf(w, "data: %s\n\n", "{\"type\":\"connected\",\"message\":\"Connected to normalization events\"}")
	flusher.Flush()

	// Слушаем события из канала
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case event := <-s.normalizerEvents:
			// Форматируем событие как JSON
			eventJSON := fmt.Sprintf("{\"type\":\"log\",\"message\":%q,\"timestamp\":%q}",
				event, time.Now().Format(time.RFC3339))
			if _, err := fmt.Fprintf(w, "data: %s\n\n", eventJSON); err != nil {
				log.Printf("Ошибка отправки SSE события: %v", err)
				return
			}
			flusher.Flush()
		case <-ticker.C:
			// Отправляем heartbeat для поддержания соединения
			if _, err := fmt.Fprintf(w, ": heartbeat\n\n"); err != nil {
				log.Printf("Ошибка отправки heartbeat: %v", err)
				return
			}
			flusher.Flush()
		case <-r.Context().Done():
			// Клиент отключился
			log.Printf("SSE клиент отключился: %v", r.Context().Err())
			return
		}
	}
}

// NormalizationStatus представляет статус процесса нормализации
type NormalizationStatus struct {
	IsRunning       bool     `json:"isRunning"`
	Progress        float64  `json:"progress"`
	Processed       int      `json:"processed"`
	Total           int      `json:"total"`
	Success         int      `json:"success,omitempty"`
	Errors          int      `json:"errors,omitempty"`
	CurrentStep     string   `json:"currentStep"`
	Logs            []string `json:"logs"`
	StartTime       string   `json:"startTime,omitempty"`
	ElapsedTime     string   `json:"elapsedTime,omitempty"`
	Rate            float64  `json:"rate,omitempty"`            // записей в секунду
	KpvedClassified int      `json:"kpvedClassified,omitempty"` // количество классифицированных групп по КПВЭД
	KpvedTotal      int      `json:"kpvedTotal,omitempty"`      // общее количество групп для КПВЭД
	KpvedProgress   float64  `json:"kpvedProgress,omitempty"`   // процент классифицированных групп по КПВЭД
}

// handleNormalizationStatus возвращает текущий статус нормализации
func (s *Server) handleNormalizationStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.normalizerMutex.RLock()
	isRunning := s.normalizerRunning
	startTime := s.normalizerStartTime
	processed := s.normalizerProcessed
	success := s.normalizerSuccess
	errors := s.normalizerErrors
	s.normalizerMutex.RUnlock()

	// Получаем реальное количество записей в catalog_items для расчета total
	var totalCatalogItems int
	err := s.db.QueryRow("SELECT COUNT(*) FROM catalog_items").Scan(&totalCatalogItems)
	if err != nil {
		// Если не удалось получить, используем значение по умолчанию
		log.Printf("Ошибка получения количества записей из catalog_items: %v", err)
		totalCatalogItems = 0
	}

	// Получаем количество записей в normalized_data
	var totalNormalized int
	err = s.db.QueryRow("SELECT COUNT(*) FROM normalized_data").Scan(&totalNormalized)
	if err != nil {
		// Таблица может не существовать или быть пустой - это нормально
		log.Printf("Ошибка получения количества нормализованных записей: %v", err)
		totalNormalized = 0
	}

	// Получаем метрики КПВЭД классификации (считаем группы, а не записи)
	// ВАЖНО: normalized_data находится в основной БД (s.db), а не в normalizedDB
	var kpvedClassified, kpvedTotal int
	var kpvedProgress float64
	if s.db != nil {
		// Считаем количество групп с классификацией КПВЭД
		err = s.db.QueryRow(`
			SELECT COUNT(DISTINCT normalized_name || '|' || category)
			FROM normalized_data
			WHERE kpved_code IS NOT NULL AND kpved_code != '' AND TRIM(kpved_code) != ''
		`).Scan(&kpvedClassified)
		if err != nil {
			log.Printf("Ошибка получения количества классифицированных по КПВЭД: %v", err)
			kpvedClassified = 0
		}

		// Считаем общее количество групп
		err = s.db.QueryRow(`
			SELECT COUNT(DISTINCT normalized_name || '|' || category)
			FROM normalized_data
		`).Scan(&kpvedTotal)
		if err != nil {
			log.Printf("Ошибка получения общего количества групп для КПВЭД: %v", err)
			kpvedTotal = 0
		}

		if kpvedTotal > 0 {
			kpvedProgress = float64(kpvedClassified) / float64(kpvedTotal) * 100
		}
	}

	// Используем processed из мьютекса, если процесс запущен, иначе из БД
	if !isRunning {
		processed = totalNormalized
	}

	// Используем реальное количество записей из catalog_items для расчета total
	// Если totalCatalogItems = 0, используем processed как total (для случая когда БД пустая)
	total := totalCatalogItems
	if total == 0 && processed > 0 {
		total = processed
	}

	// Проверяем, действительно ли процесс завершился
	progressPercent := 0.0
	if total > 0 {
		progressPercent = float64(processed) / float64(total) * 100
		if progressPercent > 100 {
			progressPercent = 100
		}
	}

	// Если processed >= total, процесс завершен
	if isRunning && total > 0 && processed >= total {
		// Завершаем процесс сразу, если все записи обработаны
		s.normalizerMutex.Lock()
		s.normalizerRunning = false
		s.normalizerMutex.Unlock()
		isRunning = false
		progressPercent = 100.0
	} else if isRunning && progressPercent >= 100 {
		// Если прогресс 100% и процесс "запущен", но нет активности - завершаем через таймаут
		if !startTime.IsZero() {
			elapsed := time.Since(startTime)
			// Если прошло более 10 секунд и прогресс 100%, считаем завершенным
			if elapsed > 10*time.Second {
				s.normalizerMutex.Lock()
				s.normalizerRunning = false
				s.normalizerMutex.Unlock()
				isRunning = false
			}
		}
	}

	status := NormalizationStatus{
		IsRunning:       isRunning,
		Progress:        progressPercent,
		Processed:       processed,
		Total:           total,
		Success:         success,
		Errors:          errors,
		CurrentStep:     "Не запущено",
		Logs:            []string{},
		KpvedClassified: kpvedClassified,
		KpvedTotal:      kpvedTotal,
		KpvedProgress:   kpvedProgress,
	}

	if isRunning {
		status.CurrentStep = "Выполняется нормализация..."

		// Добавляем время начала и прошедшее время
		if !startTime.IsZero() {
			status.StartTime = startTime.Format(time.RFC3339)
			elapsed := time.Since(startTime)
			status.ElapsedTime = elapsed.String()

			// Рассчитываем скорость обработки
			if elapsed.Seconds() > 0 {
				status.Rate = float64(processed) / elapsed.Seconds()
			}
		}
	} else if progressPercent >= 100 {
		status.CurrentStep = "Нормализация завершена"
		// Добавляем финальную статистику
		if !startTime.IsZero() {
			status.StartTime = startTime.Format(time.RFC3339)
			elapsed := time.Since(startTime)
			status.ElapsedTime = elapsed.String()
			if elapsed.Seconds() > 0 {
				status.Rate = float64(processed) / elapsed.Seconds()
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// handleNormalizationStop останавливает процесс нормализации
func (s *Server) handleNormalizationStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.normalizerMutex.Lock()
	wasRunning := s.normalizerRunning
	s.normalizerRunning = false
	s.normalizerMutex.Unlock()

	if wasRunning {
		s.normalizerEvents <- "Нормализация остановлена пользователем"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"message": "Normalization stopped",
	})
}

// handleNormalizationStats возвращает статистику нормализации
func (s *Server) handleNormalizationStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Получаем статистику из normalized_data
	// Статистика показывает количество исправленных элементов (каждая запись - это исправленный элемент
	// с разложенными по колонкам/атрибутам размерами, брендами и т.д.)
	var totalItems int
	var totalItemsWithAttributes int // Количество элементов с извлеченными атрибутами
	var lastNormalizedAt sql.NullString
	var categoryStats map[string]int = make(map[string]int)

	// Считаем все исправленные элементы (записи в normalized_data)
	err := s.db.QueryRow("SELECT COUNT(*) FROM normalized_data").Scan(&totalItems)
	if err != nil {
		log.Printf("Ошибка получения количества исправленных элементов: %v", err)
		totalItems = 0
	}

	// Считаем количество элементов, у которых есть извлеченные атрибуты (размеры, бренды и т.д.)
	err = s.db.QueryRow(`
		SELECT COUNT(DISTINCT normalized_item_id) 
		FROM normalized_item_attributes
	`).Scan(&totalItemsWithAttributes)
	if err != nil {
		// Таблица атрибутов может не существовать - это нормально
		log.Printf("Ошибка получения количества элементов с атрибутами: %v", err)
		totalItemsWithAttributes = 0
	}

	// Получаем время последней нормализации
	err = s.db.QueryRow("SELECT MAX(created_at) FROM normalized_data").Scan(&lastNormalizedAt)
	if err != nil {
		log.Printf("Ошибка получения времени последней нормализации: %v", err)
	}

	// Получаем статистику по категориям (из поля category)
	rows, err := s.db.Query(`
		SELECT 
			category,
			COUNT(*) as count
		FROM normalized_data
		WHERE category IS NOT NULL AND category != ''
		GROUP BY category
		ORDER BY count DESC
	`)
	if err != nil {
		log.Printf("Ошибка получения статистики по категориям: %v", err)
	} else {
		defer rows.Close()
		for rows.Next() {
			var category string
			var count int
			if err := rows.Scan(&category, &count); err != nil {
				log.Printf("Ошибка сканирования категории: %v", err)
				continue
			}
			categoryStats[category] = count
		}
		if err := rows.Err(); err != nil {
			log.Printf("Ошибка при итерации по категориям: %v", err)
		}
	}

	// Вычисляем количество объединенных элементов (дубликатов, которые были объединены)
	// mergedItems = общее количество - количество уникальных групп по normalized_reference
	var uniqueGroups int
	err = s.db.QueryRow("SELECT COUNT(DISTINCT normalized_reference) FROM normalized_data").Scan(&uniqueGroups)
	if err != nil {
		log.Printf("Ошибка получения количества уникальных групп: %v", err)
		uniqueGroups = 0
	}
	mergedItems := totalItems - uniqueGroups
	if mergedItems < 0 {
		mergedItems = 0
	}

	stats := map[string]interface{}{
		"totalItems":               totalItems,               // Количество исправленных элементов
		"totalItemsWithAttributes": totalItemsWithAttributes, // Количество элементов с извлеченными атрибутами
		"totalGroups":              uniqueGroups,             // Количество уникальных групп (для совместимости)
		"categories":               categoryStats,
		"mergedItems":              mergedItems, // Количество объединенных дубликатов
	}

	// Добавляем timestamp последней нормализации, если он есть
	if lastNormalizedAt.Valid && lastNormalizedAt.String != "" {
		stats["last_normalized_at"] = lastNormalizedAt.String
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// handleNormalizationGroups возвращает список уникальных групп с пагинацией
func (s *Server) handleNormalizationGroups(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Получаем параметры запроса
	query := r.URL.Query()
	pageStr := query.Get("page")
	limitStr := query.Get("limit")
	category := query.Get("category")
	search := query.Get("search")
	kpvedCode := query.Get("kpved_code")
	includeAI := query.Get("include_ai") == "true"

	// Значения по умолчанию
	page := 1
	limit := 20

	if pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	offset := (page - 1) * limit

	// Строим SQL запрос для получения уникальных групп
	baseQuery := `
		SELECT normalized_name, normalized_reference, category, COUNT(*) as merged_count`

	if includeAI {
		baseQuery += `, AVG(ai_confidence) as avg_confidence, MAX(processing_level) as processing_level`
	}

	// Всегда включаем КПВЭД поля (берем первое значение из группы)
	baseQuery += `, MAX(kpved_code) as kpved_code, MAX(kpved_name) as kpved_name, AVG(kpved_confidence) as kpved_confidence`

	// Добавляем timestamp последней нормализации для группы
	baseQuery += `, MAX(created_at) as last_normalized_at`

	baseQuery += `
		FROM normalized_data
		WHERE 1=1
	`
	countQuery := `
		SELECT COUNT(*) FROM (
			SELECT normalized_name, category
			FROM normalized_data
			WHERE 1=1
	`

	// Параметры для prepared statement
	var args []interface{}
	var countArgs []interface{}

	// Добавляем фильтр по категории
	if category != "" {
		baseQuery += " AND category = ?"
		countQuery += " AND category = ?"
		args = append(args, category)
		countArgs = append(countArgs, category)
	}

	// Добавляем поиск по нормализованному имени
	if search != "" {
		baseQuery += " AND normalized_name LIKE ?"
		countQuery += " AND normalized_name LIKE ?"
		searchParam := "%" + search + "%"
		args = append(args, searchParam)
		countArgs = append(countArgs, searchParam)
	}

	// Добавляем фильтр по КПВЭД коду
	if kpvedCode != "" {
		baseQuery += " AND kpved_code = ?"
		countQuery += " AND kpved_code = ?"
		args = append(args, kpvedCode)
		countArgs = append(countArgs, kpvedCode)
	}

	// Группировка и сортировка для основного запроса
	baseQuery += " GROUP BY normalized_name, normalized_reference, category"
	baseQuery += " ORDER BY merged_count DESC, normalized_name ASC"
	baseQuery += " LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	// Закрываем подзапрос для подсчета
	countQuery += " GROUP BY normalized_name, category)"

	// Получаем общее количество групп
	var totalGroups int
	err := s.db.QueryRow(countQuery, countArgs...).Scan(&totalGroups)
	if err != nil {
		log.Printf("Ошибка получения количества групп: %v", err)
		http.Error(w, "Failed to count groups", http.StatusInternalServerError)
		return
	}

	// Получаем группы
	rows, err := s.db.Query(baseQuery, args...)
	if err != nil {
		log.Printf("Ошибка выполнения запроса групп: %v", err)
		http.Error(w, "Failed to fetch groups", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type Group struct {
		NormalizedName      string   `json:"normalized_name"`
		NormalizedReference string   `json:"normalized_reference"`
		Category            string   `json:"category"`
		MergedCount         int      `json:"merged_count"`
		AvgConfidence       *float64 `json:"avg_confidence,omitempty"`
		ProcessingLevel     *string  `json:"processing_level,omitempty"`
		KpvedCode           *string  `json:"kpved_code,omitempty"`
		KpvedName           *string  `json:"kpved_name,omitempty"`
		KpvedConfidence     *float64 `json:"kpved_confidence,omitempty"`
		LastNormalizedAt    *string  `json:"last_normalized_at,omitempty"`
	}

	groups := []Group{}
	for rows.Next() {
		var g Group
		var lastNormalizedAt sql.NullString
		if includeAI {
			if err := rows.Scan(&g.NormalizedName, &g.NormalizedReference, &g.Category, &g.MergedCount,
				&g.AvgConfidence, &g.ProcessingLevel, &g.KpvedCode, &g.KpvedName, &g.KpvedConfidence, &lastNormalizedAt); err != nil {
				log.Printf("Ошибка сканирования группы: %v", err)
				continue
			}
		} else {
			if err := rows.Scan(&g.NormalizedName, &g.NormalizedReference, &g.Category, &g.MergedCount,
				&g.KpvedCode, &g.KpvedName, &g.KpvedConfidence, &lastNormalizedAt); err != nil {
				log.Printf("Ошибка сканирования группы: %v", err)
				continue
			}
		}
		if lastNormalizedAt.Valid && lastNormalizedAt.String != "" {
			g.LastNormalizedAt = &lastNormalizedAt.String
		}
		groups = append(groups, g)
	}

	if err := rows.Err(); err != nil {
		log.Printf("Ошибка при итерации по группам: %v", err)
	}

	// Вычисляем общее количество страниц
	totalPages := (totalGroups + limit - 1) / limit

	response := map[string]interface{}{
		"groups":     groups,
		"total":      totalGroups,
		"page":       page,
		"limit":      limit,
		"totalPages": totalPages,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleNormalizationGroupItems возвращает исходные записи для конкретной группы
func (s *Server) handleNormalizationGroupItems(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Получаем параметры запроса
	query := r.URL.Query()
	normalizedName := query.Get("normalized_name")
	category := query.Get("category")
	includeAI := query.Get("include_ai") == "true"

	if normalizedName == "" || category == "" {
		http.Error(w, "normalized_name and category are required", http.StatusBadRequest)
		return
	}

	// Запрос для получения всех исходных записей группы
	sqlQuery := `
		SELECT id, source_reference, source_name, code,
		       normalized_name, normalized_reference, category,
		       merged_count, created_at`

	if includeAI {
		sqlQuery += `, ai_confidence, ai_reasoning, processing_level`
	}

	// Всегда включаем КПВЭД поля
	sqlQuery += `, kpved_code, kpved_name, kpved_confidence`

	sqlQuery += `
		FROM normalized_data
		WHERE normalized_name = ? AND category = ?
		ORDER BY source_name
	`

	rows, err := s.db.Query(sqlQuery, normalizedName, category)
	if err != nil {
		log.Printf("Ошибка получения записей группы: %v", err)
		http.Error(w, "Failed to fetch group items", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	items := []map[string]interface{}{}
	var normalizedRef string
	var mergedCount int
	var groupKpvedCode, groupKpvedName *string
	var groupKpvedConfidence *float64

	for rows.Next() {
		var id int
		var sourceRef, sourceName, code, normName, normRef, cat string
		var mCount int
		var createdAt time.Time
		var aiConfidence *float64
		var aiReasoning *string
		var processingLevel *string
		var kpvedCode, kpvedName *string
		var kpvedConfidence *float64

		item := map[string]interface{}{}

		if includeAI {
			if err := rows.Scan(&id, &sourceRef, &sourceName, &code, &normName, &normRef, &cat, &mCount, &createdAt,
				&aiConfidence, &aiReasoning, &processingLevel, &kpvedCode, &kpvedName, &kpvedConfidence); err != nil {
				log.Printf("Ошибка сканирования записи: %v", err)
				continue
			}
			item = map[string]interface{}{
				"id":               id,
				"source_reference": sourceRef,
				"source_name":      sourceName,
				"code":             code,
				"created_at":       createdAt,
			}
			if aiConfidence != nil {
				item["ai_confidence"] = *aiConfidence
			}
			if aiReasoning != nil {
				item["ai_reasoning"] = *aiReasoning
			}
			if processingLevel != nil {
				item["processing_level"] = *processingLevel
			}
		} else {
			if err := rows.Scan(&id, &sourceRef, &sourceName, &code, &normName, &normRef, &cat, &mCount, &createdAt,
				&kpvedCode, &kpvedName, &kpvedConfidence); err != nil {
				log.Printf("Ошибка сканирования записи: %v", err)
				continue
			}
			item = map[string]interface{}{
				"id":               id,
				"source_reference": sourceRef,
				"source_name":      sourceName,
				"code":             code,
				"created_at":       createdAt,
			}
		}

		// Добавляем КПВЭД поля если они есть
		if kpvedCode != nil {
			item["kpved_code"] = *kpvedCode
		}
		if kpvedName != nil {
			item["kpved_name"] = *kpvedName
		}
		if kpvedConfidence != nil {
			item["kpved_confidence"] = *kpvedConfidence
		}

		// Сохраняем КПВЭД для группы (берем из первого элемента)
		if groupKpvedCode == nil && kpvedCode != nil {
			groupKpvedCode = kpvedCode
		}
		if groupKpvedName == nil && kpvedName != nil {
			groupKpvedName = kpvedName
		}
		if groupKpvedConfidence == nil && kpvedConfidence != nil {
			groupKpvedConfidence = kpvedConfidence
		}

		normalizedRef = normRef
		mergedCount = mCount

		// Получаем атрибуты для этого элемента
		attributes, err := s.db.GetItemAttributes(id)
		if err != nil {
			log.Printf("Ошибка получения атрибутов для элемента %d: %v", id, err)
		} else if len(attributes) > 0 {
			// Преобразуем атрибуты в JSON-совместимый формат
			attrsJSON := make([]map[string]interface{}, 0, len(attributes))
			for _, attr := range attributes {
				attrsJSON = append(attrsJSON, map[string]interface{}{
					"id":              attr.ID,
					"attribute_type":  attr.AttributeType,
					"attribute_name":  attr.AttributeName,
					"attribute_value": attr.AttributeValue,
					"unit":            attr.Unit,
					"original_text":   attr.OriginalText,
					"confidence":      attr.Confidence,
				})
			}
			item["attributes"] = attrsJSON
		}

		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		log.Printf("Ошибка при итерации по записям: %v", err)
	}

	response := map[string]interface{}{
		"normalized_name":      normalizedName,
		"normalized_reference": normalizedRef,
		"category":             category,
		"merged_count":         mergedCount,
		"items":                items,
	}

	// Добавляем КПВЭД поля на уровне группы
	if groupKpvedCode != nil {
		response["kpved_code"] = *groupKpvedCode
	}
	if groupKpvedName != nil {
		response["kpved_name"] = *groupKpvedName
	}
	if groupKpvedConfidence != nil {
		response["kpved_confidence"] = *groupKpvedConfidence
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleNormalizationItemAttributes возвращает атрибуты для конкретного нормализованного товара
func (s *Server) handleNormalizationItemAttributes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Извлекаем ID из пути /api/normalization/item-attributes/{id}
	path := r.URL.Path
	parts := strings.Split(path, "/")
	if len(parts) < 5 {
		http.Error(w, "Item ID is required", http.StatusBadRequest)
		return
	}

	itemIDStr := parts[len(parts)-1]
	itemID, err := strconv.Atoi(itemIDStr)
	if err != nil {
		http.Error(w, "Invalid item ID", http.StatusBadRequest)
		return
	}

	// Получаем атрибуты
	attributes, err := s.db.GetItemAttributes(itemID)
	if err != nil {
		log.Printf("Ошибка получения атрибутов для элемента %d: %v", itemID, err)
		http.Error(w, "Failed to fetch attributes", http.StatusInternalServerError)
		return
	}

	// Преобразуем в JSON-совместимый формат
	attrsJSON := make([]map[string]interface{}, 0, len(attributes))
	for _, attr := range attributes {
		attrsJSON = append(attrsJSON, map[string]interface{}{
			"id":              attr.ID,
			"attribute_type":  attr.AttributeType,
			"attribute_name":  attr.AttributeName,
			"attribute_value": attr.AttributeValue,
			"unit":            attr.Unit,
			"original_text":   attr.OriginalText,
			"confidence":      attr.Confidence,
			"created_at":      attr.CreatedAt,
		})
	}

	response := map[string]interface{}{
		"item_id":    itemID,
		"attributes": attrsJSON,
		"count":      len(attrsJSON),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleNormalizationExportGroup экспортирует группу в CSV или JSON формате
func (s *Server) handleNormalizationExportGroup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Получаем параметры запроса
	query := r.URL.Query()
	normalizedName := query.Get("normalized_name")
	category := query.Get("category")
	format := query.Get("format")

	if normalizedName == "" || category == "" {
		http.Error(w, "normalized_name and category are required", http.StatusBadRequest)
		return
	}

	// По умолчанию CSV формат
	if format == "" {
		format = "csv"
	}

	// Получаем данные группы
	sqlQuery := `
		SELECT id, source_reference, source_name, code,
		       normalized_name, normalized_reference, category,
		       created_at, ai_confidence, ai_reasoning, processing_level
		FROM normalized_data
		WHERE normalized_name = ? AND category = ?
		ORDER BY source_name
	`

	rows, err := s.db.Query(sqlQuery, normalizedName, category)
	if err != nil {
		log.Printf("Ошибка получения записей группы для экспорта: %v", err)
		http.Error(w, "Failed to fetch group items", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	type ExportItem struct {
		ID                  int       `json:"id"`
		Code                string    `json:"code"`
		SourceName          string    `json:"source_name"`
		SourceReference     string    `json:"source_reference"`
		NormalizedName      string    `json:"normalized_name"`
		NormalizedReference string    `json:"normalized_reference"`
		Category            string    `json:"category"`
		CreatedAt           time.Time `json:"created_at"`
		AIConfidence        *float64  `json:"ai_confidence,omitempty"`
		AIReasoning         *string   `json:"ai_reasoning,omitempty"`
		ProcessingLevel     *string   `json:"processing_level,omitempty"`
	}

	items := []ExportItem{}
	for rows.Next() {
		var item ExportItem
		if err := rows.Scan(
			&item.ID,
			&item.SourceReference,
			&item.SourceName,
			&item.Code,
			&item.NormalizedName,
			&item.NormalizedReference,
			&item.Category,
			&item.CreatedAt,
			&item.AIConfidence,
			&item.AIReasoning,
			&item.ProcessingLevel,
		); err != nil {
			log.Printf("Ошибка сканирования записи для экспорта: %v", err)
			continue
		}
		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		log.Printf("Ошибка при итерации по записям: %v", err)
	}

	// Формируем имя файла
	timestamp := time.Now().Format("2006-01-02")
	filename := fmt.Sprintf("group_%s_%s_%s.%s", normalizedName, category, timestamp, format)

	if format == "csv" {
		// Экспорт в CSV
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))

		// UTF-8 BOM для корректного отображения в Excel
		w.Write([]byte{0xEF, 0xBB, 0xBF})

		writer := csv.NewWriter(w)
		defer writer.Flush()

		// Заголовки
		headers := []string{
			"ID", "Код", "Исходное название", "Исходный reference",
			"Нормализованное название", "Нормализованный reference",
			"Категория", "AI Confidence", "Processing Level", "Дата создания",
		}
		writer.Write(headers)

		// Данные
		for _, item := range items {
			confidence := ""
			if item.AIConfidence != nil {
				confidence = fmt.Sprintf("%.2f", *item.AIConfidence)
			}

			processingLevel := ""
			if item.ProcessingLevel != nil {
				processingLevel = *item.ProcessingLevel
			}

			record := []string{
				fmt.Sprintf("%d", item.ID),
				item.Code,
				item.SourceName,
				item.SourceReference,
				item.NormalizedName,
				item.NormalizedReference,
				item.Category,
				confidence,
				processingLevel,
				item.CreatedAt.Format("2006-01-02 15:04:05"),
			}
			writer.Write(record)
		}
	} else if format == "json" {
		// Экспорт в JSON
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))

		exportData := map[string]interface{}{
			"group_name":  normalizedName,
			"category":    category,
			"export_date": time.Now().Format(time.RFC3339),
			"item_count":  len(items),
			"items":       items,
		}

		json.NewEncoder(w).Encode(exportData)
	} else {
		http.Error(w, "Invalid format. Supported formats: csv, json", http.StatusBadRequest)
	}
}

// handleDatabaseInfo возвращает информацию о текущей базе данных
func (s *Server) handleDatabaseInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.dbMutex.RLock()
	defer s.dbMutex.RUnlock()

	// Получаем статистику из БД
	stats, err := s.db.GetStats()
	if err != nil {
		log.Printf("Ошибка получения статистики БД: %v", err)
		http.Error(w, "Failed to fetch database stats", http.StatusInternalServerError)
		return
	}

	// Получаем информацию о файле
	fileInfo, err := os.Stat(s.currentDBPath)
	var fileSize int64
	var modTime time.Time
	if err == nil {
		fileSize = fileInfo.Size()
		modTime = fileInfo.ModTime()
	}

	response := map[string]interface{}{
		"name":        filepath.Base(s.currentDBPath),
		"path":        s.currentDBPath,
		"size":        fileSize,
		"modified_at": modTime,
		"stats":       stats,
		"status":      "connected",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleDatabasesList возвращает список доступных баз данных
func (s *Server) handleDatabasesList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Сканируем несколько директорий на наличие .db файлов
	var allFiles []string

	// 1. Текущая директория
	files, err := filepath.Glob("*.db")
	if err == nil {
		allFiles = append(allFiles, files...)
	}

	// 2. Директория /app/data (для Docker)
	dataFiles, err := filepath.Glob("data/*.db")
	if err == nil {
		for _, file := range dataFiles {
			allFiles = append(allFiles, file)
		}
	}

	// 3. Директория /app/data (абсолютный путь для Docker)
	absDataFiles, err := filepath.Glob("/app/data/*.db")
	if err == nil {
		for _, file := range absDataFiles {
			allFiles = append(allFiles, file)
		}
	}

	// 4. Директория /app (абсолютный путь для Docker)
	absAppFiles, err := filepath.Glob("/app/*.db")
	if err == nil {
		for _, file := range absAppFiles {
			// Пропускаем service.db из корня, так как он должен быть в data/
			if filepath.Base(file) != "service.db" || filepath.Dir(file) == "/app/data" {
				allFiles = append(allFiles, file)
			}
		}
	}

	// Убираем дубликаты по абсолютному пути
	fileMap := make(map[string]string) // absPath -> original path
	uniqueFiles := []string{}
	for _, file := range allFiles {
		absPath, err := filepath.Abs(file)
		if err != nil {
			absPath = file
		}
		// Нормализуем путь (убираем лишние слеши и т.д.)
		absPath = filepath.Clean(absPath)

		// Если файл уже есть, выбираем более короткий путь
		if existingPath, exists := fileMap[absPath]; exists {
			// Предпочитаем путь из /app/data/ или более короткий
			if len(file) < len(existingPath) || strings.Contains(file, "data/") {
				fileMap[absPath] = file
			}
		} else {
			fileMap[absPath] = file
			uniqueFiles = append(uniqueFiles, file)
		}
	}

	// Обновляем список уникальных файлов с выбранными путями
	uniqueFiles = []string{}
	for _, path := range fileMap {
		uniqueFiles = append(uniqueFiles, path)
	}

	databases := []map[string]interface{}{}
	s.dbMutex.RLock()
	currentDB := s.currentDBPath
	s.dbMutex.RUnlock()

	for _, file := range uniqueFiles {
		fileInfo, err := os.Stat(file)
		if err != nil {
			continue
		}

		isCurrent := file == currentDB

		// Определяем тип базы данных
		dbType, err := database.DetectDatabaseType(file)
		if err != nil {
			log.Printf("Ошибка определения типа БД %s: %v", file, err)
			dbType = "unknown"
		}

		// Получаем метаданные из serviceDB
		var metadata *database.DatabaseMetadata
		if s.serviceDB != nil {
			metadata, _ = s.serviceDB.GetDatabaseMetadata(file)
		}

		// Если метаданных нет, создаем их
		if metadata == nil && s.serviceDB != nil {
			description := fmt.Sprintf("База данных типа %s", dbType)
			s.serviceDB.UpsertDatabaseMetadata(file, dbType, description, "{}")
			metadata, _ = s.serviceDB.GetDatabaseMetadata(file)
		}

		dbInfo := map[string]interface{}{
			"name":        file,
			"path":        file,
			"size":        fileInfo.Size(),
			"modified_at": fileInfo.ModTime(),
			"is_current":  isCurrent,
			"type":        dbType,
		}

		// Добавляем информацию из метаданных
		if metadata != nil {
			dbInfo["first_seen_at"] = metadata.FirstSeenAt
			dbInfo["last_analyzed_at"] = metadata.LastAnalyzedAt
			dbInfo["description"] = metadata.Description
		}

		// Получаем базовую статистику (количество таблиц)
		tableStats, err := database.GetTableStats(file)
		if err == nil {
			var totalRows int64
			for _, stat := range tableStats {
				totalRows += stat.RowCount
			}
			dbInfo["table_count"] = len(tableStats)
			dbInfo["total_rows"] = totalRows
		}

		databases = append(databases, dbInfo)
	}

	response := map[string]interface{}{
		"databases": databases,
		"current":   currentDB,
		"total":     len(databases),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleDatabaseAnalytics возвращает детальную аналитику базы данных
func (s *Server) handleDatabaseAnalytics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Приоритет 1: путь из query параметра
	dbPath := r.URL.Query().Get("path")

	// Приоритет 2: имя из пути URL (для обратной совместимости)
	if dbPath == "" {
		path := r.URL.Path
		dbPath = strings.TrimPrefix(path, "/api/databases/analytics/")
		// Убираем завершающий слеш, если есть
		dbPath = strings.TrimSuffix(dbPath, "/")
	}

	if dbPath == "" {
		s.writeJSONError(w, "Database path is required", http.StatusBadRequest)
		return
	}

	// Декодируем путь, если он был закодирован
	if decodedPath, err := url.QueryUnescape(dbPath); err == nil {
		dbPath = decodedPath
	}

	// Логируем запрос для отладки
	log.Printf("Запрос аналитики для БД: %s", dbPath)

	// Проверяем существование файла
	_, err := os.Stat(dbPath)
	if os.IsNotExist(err) {
		wd, _ := os.Getwd()
		log.Printf("Файл БД не найден: %s (текущая директория: %s)", dbPath, wd)
		s.writeJSONError(w, fmt.Sprintf("Database file not found: %s", dbPath), http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("Ошибка проверки файла БД %s: %v", dbPath, err)
		s.writeJSONError(w, fmt.Sprintf("Error checking database file: %v", err), http.StatusInternalServerError)
		return
	}

	// Получаем аналитику
	analytics, err := database.GetDatabaseAnalytics(dbPath)
	if err != nil {
		log.Printf("Ошибка получения аналитики БД %s: %v", dbPath, err)
		s.writeJSONError(w, fmt.Sprintf("Failed to get analytics: %v", err), http.StatusInternalServerError)
		return
	}

	// Обновляем историю изменений
	if s.serviceDB != nil {
		var totalRows int64
		for _, stat := range analytics.TableStats {
			totalRows += stat.RowCount
		}
		database.UpdateDatabaseHistory(s.serviceDB, dbPath, analytics.TotalSize, totalRows)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(analytics)
}

// handleDatabaseHistory возвращает историю изменений базы данных
func (s *Server) handleDatabaseHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.serviceDB == nil {
		s.writeJSONError(w, "Service database not available", http.StatusInternalServerError)
		return
	}

	// Извлекаем имя базы данных из пути
	path := r.URL.Path
	dbName := strings.TrimPrefix(path, "/api/databases/history/")
	if dbName == "" {
		s.writeJSONError(w, "Database name is required", http.StatusBadRequest)
		return
	}

	// Получаем историю
	history, err := database.GetDatabaseHistory(s.serviceDB, dbName)
	if err != nil {
		log.Printf("Ошибка получения истории БД %s: %v", dbName, err)
		s.writeJSONError(w, fmt.Sprintf("Failed to get history: %v", err), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"database": dbName,
		"history":  history,
		"count":    len(history),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleFindDatabase ищет database_id по client_id и project_id
func (s *Server) handleFindDatabase(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	clientIDStr := r.URL.Query().Get("client_id")
	projectIDStr := r.URL.Query().Get("project_id")

	if clientIDStr == "" || projectIDStr == "" {
		s.writeJSONError(w, "client_id and project_id are required", http.StatusBadRequest)
		return
	}

	clientID, err := strconv.Atoi(clientIDStr)
	if err != nil {
		s.writeJSONError(w, "Invalid client_id", http.StatusBadRequest)
		return
	}

	projectID, err := strconv.Atoi(projectIDStr)
	if err != nil {
		s.writeJSONError(w, "Invalid project_id", http.StatusBadRequest)
		return
	}

	if s.serviceDB == nil {
		s.writeJSONError(w, "Service database not available", http.StatusInternalServerError)
		return
	}

	// Проверяем существование проекта и принадлежность клиенту
	project, err := s.serviceDB.GetClientProject(projectID)
	if err != nil {
		s.writeJSONError(w, "Project not found", http.StatusNotFound)
		return
	}

	if project.ClientID != clientID {
		s.writeJSONError(w, "Project does not belong to this client", http.StatusBadRequest)
		return
	}

	// Получаем список баз данных проекта (сначала активные)
	databases, err := s.serviceDB.GetProjectDatabases(projectID, true)
	if err != nil {
		s.writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Если активных нет, получаем все
	if len(databases) == 0 {
		databases, err = s.serviceDB.GetProjectDatabases(projectID, false)
		if err != nil {
			s.writeJSONError(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	if len(databases) == 0 {
		s.writeJSONError(w, "No databases found for this project", http.StatusNotFound)
		return
	}

	// Возвращаем первую базу данных
	db := databases[0]
	response := map[string]interface{}{
		"database_id": db.ID,
		"name":        db.Name,
		"exists":      true,
		"is_active":   db.IsActive,
		"file_path":   db.FilePath,
	}

	s.writeJSONResponse(w, response, http.StatusOK)
}

// handleDatabaseSwitch переключает текущую базу данных
func (s *Server) handleDatabaseSwitch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Проверяем, что нормализация не запущена
	s.normalizerMutex.RLock()
	isRunning := s.normalizerRunning
	s.normalizerMutex.RUnlock()

	if isRunning {
		http.Error(w, "Cannot switch database while normalization is running", http.StatusBadRequest)
		return
	}

	// Читаем запрос
	var request struct {
		Path string `json:"path"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if request.Path == "" {
		http.Error(w, "Database path is required", http.StatusBadRequest)
		return
	}

	// Проверяем, что файл существует
	if _, err := os.Stat(request.Path); os.IsNotExist(err) {
		http.Error(w, "Database file not found", http.StatusNotFound)
		return
	}

	s.dbMutex.Lock()
	defer s.dbMutex.Unlock()

	// Закрываем текущую БД
	if err := s.db.Close(); err != nil {
		log.Printf("Ошибка закрытия текущей БД: %v", err)
		http.Error(w, "Failed to close current database", http.StatusInternalServerError)
		return
	}

	// Открываем новую БД
	newDB, err := database.NewDB(request.Path)
	if err != nil {
		log.Printf("Ошибка открытия новой БД: %v", err)
		// Пытаемся восстановить старую БД
		oldDB, restoreErr := database.NewDB(s.currentDBPath)
		if restoreErr != nil {
			log.Printf("КРИТИЧЕСКАЯ ОШИБКА: не удалось восстановить старую БД: %v", restoreErr)
			http.Error(w, "Failed to open new database and restore failed", http.StatusInternalServerError)
			return
		}
		s.db = oldDB
		http.Error(w, "Failed to open new database", http.StatusInternalServerError)
		return
	}

	// Успешно переключились
	s.db = newDB
	s.currentDBPath = request.Path

	log.Printf("База данных переключена на: %s", request.Path)

	response := map[string]interface{}{
		"status":  "success",
		"message": "Database switched successfully",
		"path":    request.Path,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// getNomenclatureStatus возвращает статус обработки номенклатуры
func (s *Server) getNomenclatureStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Получаем статистику из БД
	dbStats, err := s.getNomenclatureDBStats(s.normalizedDB)
	if err != nil {
		s.writeJSONError(w, fmt.Sprintf("Failed to get DB stats: %v", err), http.StatusInternalServerError)
		return
	}

	// Получаем статистику из процессора, если он запущен
	var response NomenclatureStatusResponse
	response.DBStats = dbStats

	s.processorMutex.RLock()
	processor := s.nomenclatureProcessor
	s.processorMutex.RUnlock()

	if processor != nil {
		stats := processor.GetStats()
		if stats != nil && stats.Total > 0 {
			// Проверяем, идет ли обработка
			// Обработка активна, если:
			// 1. Есть необработанные записи (Processed < Total)
			// 2. Время начала установлено
			// 3. С момента начала прошло не более 5 минут без обновлений (защита от зависших процессов)
			isProcessing := stats.Processed < stats.Total && !stats.StartTime.IsZero()

			// Дополнительная проверка: если прошло более 5 минут без обновлений, считаем обработку завершенной
			if isProcessing {
				elapsed := time.Since(stats.StartTime)
				// Если прошло более 5 минут и нет прогресса, считаем обработку завершенной
				if elapsed > 5*time.Minute && stats.Processed == 0 {
					isProcessing = false
				}
			}

			response.Processing = isProcessing
			response.CurrentStats = &ProcessingStatsResponse{
				Total:      stats.Total,
				Processed:  stats.Processed,
				Successful: stats.Successful,
				Failed:     stats.Failed,
				StartTime:  stats.StartTime,
				MaxWorkers: processor.GetConfig().MaxWorkers,
			}
		} else {
			response.Processing = false
		}
	} else {
		response.Processing = false
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// getProcessingStatus возвращает статус обработки номенклатуры (старый метод для совместимости)
func (s *Server) getProcessingStatus(w http.ResponseWriter, r *http.Request) {
	s.getNomenclatureStatus(w, r)
}

// getNomenclatureRecentRecords возвращает последние обработанные записи
func (s *Server) getNomenclatureRecentRecords(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Получаем параметр limit из запроса (по умолчанию 15)
	limit := 15
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 && parsedLimit <= 100 {
			limit = parsedLimit
		}
	}

	// Запрос для получения последних записей
	query := `
		SELECT id, name, normalized_name, kpved_code, kpved_name, 
		       processing_status, processed_at
		FROM catalog_items
		WHERE processing_status IN ('completed', 'error')
		ORDER BY COALESCE(processed_at, last_processed_at, created_at) DESC
		LIMIT ?
	`

	// Используем Query для получения нескольких строк
	rows, err := s.normalizedDB.Query(query, limit)
	if err != nil {
		s.writeJSONError(w, fmt.Sprintf("Failed to get recent records: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var records []RecentRecord
	for rows.Next() {
		var record RecentRecord
		var processedAtStr sql.NullString

		err := rows.Scan(
			&record.ID,
			&record.OriginalName,
			&record.NormalizedName,
			&record.KpvedCode,
			&record.KpvedName,
			&record.Status,
			&processedAtStr,
		)
		if err != nil {
			log.Printf("Error scanning recent record: %v", err)
			continue
		}

		// Парсим время обработки, если оно есть
		if processedAtStr.Valid && processedAtStr.String != "" {
			if parsedTime, err := time.Parse(time.RFC3339, processedAtStr.String); err == nil {
				record.ProcessedAt = &parsedTime
			}
		}

		records = append(records, record)
	}

	if err = rows.Err(); err != nil {
		s.writeJSONError(w, fmt.Sprintf("Error iterating recent records: %v", err), http.StatusInternalServerError)
		return
	}

	response := RecentRecordsResponse{
		Records: records,
		Total:   len(records),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// getNomenclaturePendingRecords возвращает необработанные записи
func (s *Server) getNomenclaturePendingRecords(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Получаем параметры из запроса
	limit := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 && parsedLimit <= 500 {
			limit = parsedLimit
		}
	}

	offset := 0
	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if parsedOffset, err := strconv.Atoi(offsetStr); err == nil && parsedOffset >= 0 {
			offset = parsedOffset
		}
	}

	// Запрос для получения необработанных записей
	query := `
		SELECT id, name, 
		       COALESCE(processing_status, 'pending') as status,
		       created_at
		FROM catalog_items
		WHERE processing_status IS NULL OR processing_status = 'pending'
		ORDER BY id ASC
		LIMIT ? OFFSET ?
	`

	rows, err := s.normalizedDB.Query(query, limit, offset)
	if err != nil {
		s.writeJSONError(w, fmt.Sprintf("Failed to get pending records: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var records []PendingRecord
	for rows.Next() {
		var record PendingRecord
		var createdAtStr string

		err := rows.Scan(
			&record.ID,
			&record.OriginalName,
			&record.Status,
			&createdAtStr,
		)
		if err != nil {
			log.Printf("Error scanning pending record: %v", err)
			continue
		}

		// Парсим время создания
		if parsedTime, err := time.Parse(time.RFC3339, createdAtStr); err == nil {
			record.CreatedAt = parsedTime
		} else if parsedTime, err := time.Parse("2006-01-02 15:04:05", createdAtStr); err == nil {
			record.CreatedAt = parsedTime
		} else {
			record.CreatedAt = time.Now()
		}

		records = append(records, record)
	}

	if err = rows.Err(); err != nil {
		s.writeJSONError(w, fmt.Sprintf("Error iterating pending records: %v", err), http.StatusInternalServerError)
		return
	}

	// Получаем общее количество необработанных записей
	var total int
	err = s.normalizedDB.QueryRow("SELECT COUNT(*) FROM catalog_items WHERE processing_status IS NULL OR processing_status = 'pending'").Scan(&total)
	if err != nil {
		log.Printf("Error getting total pending count: %v", err)
		total = len(records)
	}

	response := PendingRecordsResponse{
		Records: records,
		Total:   total,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// serveNomenclatureStatusPage отдает HTML страницу мониторинга
func (s *Server) serveNomenclatureStatusPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Пытаемся прочитать файл из static директории
	htmlContent, err := os.ReadFile("./static/nomenclature_status.html")
	if err != nil {
		// Если файл не найден, используем встроенную версию
		htmlContent = []byte(getNomenclatureStatusPageHTML())
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(htmlContent)
}

// getNomenclatureStatusPageHTML возвращает встроенную HTML страницу мониторинга
func getNomenclatureStatusPageHTML() string {
	return `<!DOCTYPE html>
<html lang="ru">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Мониторинг обработки номенклатуры</title>
    <link href="https://cdn.jsdelivr.net/npm/bootstrap@5.3.0/dist/css/bootstrap.min.css" rel="stylesheet">
    <link rel="stylesheet" href="https://cdnjs.cloudflare.com/ajax/libs/font-awesome/6.4.0/css/all.min.css">
    <script src="https://cdn.jsdelivr.net/npm/chart.js"></script>
    <style>
        .card { transition: transform 0.2s; height: 100%; }
        .card:hover { transform: translateY(-5px); }
        .progress { height: 10px; }
        #progressChart { max-height: 300px; }
        .table-responsive { max-height: 400px; }
        .last-updated { font-size: 0.8rem; color: #6c757d; }
        .thread-status { display: flex; align-items: center; margin-bottom: 10px; }
        .thread-indicator { width: 12px; height: 12px; border-radius: 50%; margin-right: 10px; }
        .thread-active { background-color: #28a745; animation: pulse 1.5s infinite; }
        .thread-idle { background-color: #6c757d; }
        @keyframes pulse { 0% { opacity: 1; } 50% { opacity: 0.5; } 100% { opacity: 1; } }
        .dark-mode { background-color: #212529; color: #f8f9fa; }
        .dark-mode .card { background-color: #343a40; color: #f8f9fa; }
        .dark-mode .table { color: #f8f9fa; }
    </style>
</head>
<body>
    <div class="container-fluid py-4">
        <div class="d-flex justify-content-between align-items-center mb-4">
            <h1 class="h3"><i class="fas fa-database me-2"></i>Мониторинг обработки номенклатуры</h1>
            <div>
                <button class="btn btn-sm btn-outline-secondary me-2" id="themeToggle"><i class="fas fa-moon"></i> Тема</button>
                <button class="btn btn-sm btn-outline-primary" id="refreshBtn"><i class="fas fa-sync-alt"></i> Обновить</button>
            </div>
        </div>
        <div class="row mb-4">
            <div class="col-md-3 col-sm-6 mb-3">
                <div class="card bg-primary text-white">
                    <div class="card-body">
                        <div class="d-flex justify-content-between">
                            <div><h4 class="card-title" id="totalRecords">0</h4><p class="card-text">Всего записей</p></div>
                            <div class="align-self-center"><i class="fas fa-table fa-2x"></i></div>
                        </div>
                    </div>
                </div>
            </div>
            <div class="col-md-3 col-sm-6 mb-3">
                <div class="card bg-success text-white">
                    <div class="card-body">
                        <div class="d-flex justify-content-between">
                            <div><h4 class="card-title" id="processedRecords">0</h4><p class="card-text">Обработано</p></div>
                            <div class="align-self-center"><i class="fas fa-check-circle fa-2x"></i></div>
                        </div>
                    </div>
                </div>
            </div>
            <div class="col-md-3 col-sm-6 mb-3">
                <div class="card bg-warning text-dark">
                    <div class="card-body">
                        <div class="d-flex justify-content-between">
                            <div><h4 class="card-title" id="pendingRecords">0</h4><p class="card-text">Ожидают обработки</p></div>
                            <div class="align-self-center"><i class="fas fa-clock fa-2x"></i></div>
                        </div>
                    </div>
                </div>
            </div>
            <div class="col-md-3 col-sm-6 mb-3">
                <div class="card bg-danger text-white">
                    <div class="card-body">
                        <div class="d-flex justify-content-between">
                            <div><h4 class="card-title" id="errorRecords">0</h4><p class="card-text">С ошибками</p></div>
                            <div class="align-self-center"><i class="fas fa-exclamation-circle fa-2x"></i></div>
                        </div>
                    </div>
                </div>
            </div>
        </div>
        <div class="row">
            <div class="col-lg-8">
                <div class="card mb-4">
                    <div class="card-header d-flex justify-content-between align-items-center">
                        <h5 class="card-title mb-0"><i class="fas fa-tasks me-2"></i>Статус обработки</h5>
                        <span class="badge bg-success d-none" id="processingBadge">Активна</span>
                        <span class="badge bg-secondary" id="idleBadge">Не активна</span>
                    </div>
                    <div class="card-body">
                        <div class="mb-3">
                            <div class="d-flex justify-content-between mb-1">
                                <span>Прогресс обработки</span>
                                <span id="progressPercent">0%</span>
                            </div>
                            <div class="progress">
                                <div id="progressBar" class="progress-bar progress-bar-striped progress-bar-animated" role="progressbar" style="width: 0%"></div>
                            </div>
                            <div class="text-center mt-2">
                                <span id="progressText">Обработано <strong>0</strong> из <strong>0</strong> записей</span>
                            </div>
                        </div>
                        <div class="row">
                            <div class="col-md-4">
                                <div class="card bg-light">
                                    <div class="card-body text-center py-3">
                                        <h6>Время начала</h6>
                                        <p class="mb-0" id="startTime">-</p>
                                    </div>
                                </div>
                            </div>
                            <div class="col-md-4">
                                <div class="card bg-light">
                                    <div class="card-body text-center py-3">
                                        <h6>Прошедшее время</h6>
                                        <p class="mb-0" id="elapsedTime">-</p>
                                    </div>
                                </div>
                            </div>
                            <div class="col-md-4">
                                <div class="card bg-light">
                                    <div class="card-body text-center py-3">
                                        <h6>Оставшееся время</h6>
                                        <p class="mb-0" id="remainingTime">-</p>
                                    </div>
                                </div>
                            </div>
                        </div>
                    </div>
                </div>
                <div class="card mb-4">
                    <div class="card-header">
                        <h5 class="card-title mb-0"><i class="fas fa-chart-line me-2"></i>График прогресса обработки</h5>
                    </div>
                    <div class="card-body">
                        <canvas id="progressChart"></canvas>
                    </div>
                </div>
            </div>
            <div class="col-lg-4">
                <div class="card mb-4">
                    <div class="card-header">
                        <h5 class="card-title mb-0"><i class="fas fa-microchip me-2"></i>Потоки обработки</h5>
                    </div>
                    <div class="card-body">
                        <div class="thread-status">
                            <div class="thread-indicator thread-idle" id="thread1Indicator"></div>
                            <div><strong>Поток 1</strong><div class="text-muted small" id="thread1Status">Ожидание</div></div>
                        </div>
                        <div class="thread-status">
                            <div class="thread-indicator thread-idle" id="thread2Indicator"></div>
                            <div><strong>Поток 2</strong><div class="text-muted small" id="thread2Status">Ожидание</div></div>
                        </div>
                        <div class="mt-3">
                            <div class="d-flex justify-content-between">
                                <span>Скорость обработки:</span>
                                <span id="processingSpeed">0 записей/мин</span>
                            </div>
                        </div>
                    </div>
                </div>
                <div class="card mb-4">
                    <div class="card-header">
                        <h5 class="card-title mb-0"><i class="fas fa-cogs me-2"></i>Управление обработкой</h5>
                    </div>
                    <div class="card-body">
                        <div class="d-grid gap-2">
                            <button class="btn btn-success" id="startBtn"><i class="fas fa-play me-2"></i>Запустить обработку</button>
                            <button class="btn btn-warning" id="stopBtn" disabled><i class="fas fa-stop me-2"></i>Остановить обработку</button>
                        </div>
                        <div class="mt-3">
                            <div class="form-check form-switch">
                                <input class="form-check-input" type="checkbox" id="autoRefresh" checked>
                                <label class="form-check-label" for="autoRefresh">Автообновление (каждые 5 сек)</label>
                            </div>
                        </div>
                    </div>
                </div>
            </div>
        </div>
        <div class="card mb-4">
            <div class="card-header d-flex justify-content-between align-items-center">
                <h5 class="card-title mb-0"><i class="fas fa-clock me-2"></i>Необработанные номенклатуры</h5>
                <div>
                    <span class="badge bg-warning me-2" id="pendingCountBadge">0 записей</span>
                    <button class="btn btn-sm btn-success" id="startProcessingBtn"><i class="fas fa-play me-1"></i>Запустить обработку</button>
                </div>
            </div>
            <div class="card-body">
                <div class="table-responsive">
                    <table class="table table-striped table-hover">
                        <thead>
                            <tr>
                                <th>ID</th>
                                <th>Наименование</th>
                                <th>Статус</th>
                                <th>Дата создания</th>
                            </tr>
                        </thead>
                        <tbody id="pendingRecords">
                            <tr><td colspan="4" class="text-center text-muted">Загрузка данных...</td></tr>
                        </tbody>
                    </table>
                </div>
                <div class="mt-3 d-flex justify-content-between align-items-center">
                    <div>
                        <button class="btn btn-sm btn-outline-secondary" id="prevPageBtn" disabled><i class="fas fa-chevron-left"></i> Назад</button>
                        <span class="mx-2" id="pageInfo">Страница 1</span>
                        <button class="btn btn-sm btn-outline-secondary" id="nextPageBtn">Вперед <i class="fas fa-chevron-right"></i></button>
                    </div>
                    <div>
                        <span class="text-muted small">Показано: <span id="pendingShown">0</span> из <span id="pendingTotal">0</span></span>
                    </div>
                </div>
            </div>
        </div>
        <div class="card">
            <div class="card-header d-flex justify-content-between align-items-center">
                <h5 class="card-title mb-0"><i class="fas fa-history me-2"></i>Последние обработанные записи</h5>
                <span class="last-updated">Обновлено: <span id="lastUpdateTime">-</span></span>
            </div>
            <div class="card-body">
                <div class="table-responsive">
                    <table class="table table-striped table-hover">
                        <thead>
                            <tr>
                                <th>ID</th>
                                <th>Исходное наименование</th>
                                <th>Нормализованное наименование</th>
                                <th>Код КПВЭД</th>
                                <th>Статус</th>
                                <th>Время обработки</th>
                            </tr>
                        </thead>
                        <tbody id="recentRecords">
                            <tr><td colspan="6" class="text-center text-muted">Загрузка данных...</td></tr>
                        </tbody>
                    </table>
                </div>
            </div>
        </div>
    </div>
    <script>
        let refreshInterval, progressChart, processingActive = false, progressData = { labels: [], values: [] };
        let pendingPage = 0;
        const pendingPageSize = 50;
        document.addEventListener('DOMContentLoaded', function() {
            initializeChart();
            loadData();
            setupEventListeners();
            startAutoRefresh();
        });
        function initializeChart() {
            const ctx = document.getElementById('progressChart').getContext('2d');
            progressChart = new Chart(ctx, {
                type: 'line',
                data: { labels: progressData.labels, datasets: [{
                    label: 'Прогресс обработки (%)',
                    data: progressData.values,
                    borderColor: 'rgb(75, 192, 192)',
                    backgroundColor: 'rgba(75, 192, 192, 0.1)',
                    tension: 0.3,
                    fill: true
                }]},
                options: {
                    responsive: true,
                    maintainAspectRatio: false,
                    scales: { y: { beginAtZero: true, max: 100, ticks: { callback: function(value) { return value + '%'; } } } }
                }
            });
        }
        function setupEventListeners() {
            document.getElementById('refreshBtn').addEventListener('click', loadData);
            document.getElementById('startBtn').addEventListener('click', startProcessing);
            document.getElementById('startProcessingBtn').addEventListener('click', startProcessing);
            document.getElementById('stopBtn').addEventListener('click', stopProcessing);
            document.getElementById('themeToggle').addEventListener('click', toggleTheme);
            document.getElementById('autoRefresh').addEventListener('change', toggleAutoRefresh);
            document.getElementById('prevPageBtn').addEventListener('click', () => {
                if (pendingPage > 0) {
                    pendingPage--;
                    loadPendingRecords();
                }
            });
            document.getElementById('nextPageBtn').addEventListener('click', () => {
                pendingPage++;
                loadPendingRecords();
            });
        }
        function loadData() {
            fetch('/api/nomenclature/status')
                .then(response => response.json())
                .then(data => updateUI(data))
                .catch(error => { console.error('Ошибка загрузки данных:', error); showError('Не удалось загрузить данные'); });
            loadRecentRecords();
            loadPendingRecords();
        }
        function loadPendingRecords() {
            const offset = pendingPage * pendingPageSize;
            fetch('/api/nomenclature/pending?limit=' + pendingPageSize + '&offset=' + offset)
                .then(response => {
                    if (!response.ok) throw new Error('Ошибка загрузки необработанных записей');
                    return response.json();
                })
                .then(data => {
                    updatePendingRecordsTable(data.records);
                    updatePendingPagination(data.total, data.records.length);
                })
                .catch(error => {
                    console.error('Ошибка загрузки необработанных записей:', error);
                    const tbody = document.getElementById('pendingRecords');
                    tbody.innerHTML = '<tr><td colspan="4" class="text-center text-danger">Ошибка загрузки данных</td></tr>';
                });
        }
        function updatePendingRecordsTable(records) {
            const tbody = document.getElementById('pendingRecords');
            if (!records || records.length === 0) {
                tbody.innerHTML = '<tr><td colspan="4" class="text-center text-muted">Нет необработанных записей</td></tr>';
                return;
            }
            tbody.innerHTML = '';
            records.forEach(record => {
                const statusBadge = '<span class="badge bg-warning">Ожидает обработки</span>';
                const createdAt = new Date(record.created_at).toLocaleString();
                const row = document.createElement('tr');
                row.innerHTML = '<td>' + record.id + '</td>' +
                    '<td>' + escapeHtml(record.original_name || '-') + '</td>' +
                    '<td>' + statusBadge + '</td>' +
                    '<td>' + createdAt + '</td>';
                tbody.appendChild(row);
            });
        }
        function updatePendingPagination(total, shown) {
            document.getElementById('pendingTotal').textContent = total;
            document.getElementById('pendingShown').textContent = shown;
            document.getElementById('pendingCountBadge').textContent = total + ' записей';
            const totalPages = Math.ceil(total / pendingPageSize);
            const currentPage = pendingPage + 1;
            document.getElementById('pageInfo').textContent = 'Страница ' + currentPage + ' из ' + (totalPages || 1);
            document.getElementById('prevPageBtn').disabled = pendingPage === 0;
            document.getElementById('nextPageBtn').disabled = currentPage >= totalPages || shown < pendingPageSize;
        }
        function loadRecentRecords() {
            fetch('/api/nomenclature/recent?limit=15')
                .then(response => {
                    if (!response.ok) throw new Error('Ошибка загрузки последних записей');
                    return response.json();
                })
                .then(data => updateRecentRecordsTable(data.records))
                .catch(error => {
                    console.error('Ошибка загрузки последних записей:', error);
                    const tbody = document.getElementById('recentRecords');
                    tbody.innerHTML = '<tr><td colspan="6" class="text-center text-danger">Ошибка загрузки данных</td></tr>';
                });
        }
        function updateRecentRecordsTable(records) {
            const tbody = document.getElementById('recentRecords');
            if (!records || records.length === 0) {
                tbody.innerHTML = '<tr><td colspan="6" class="text-center text-muted">Нет обработанных записей</td></tr>';
                return;
            }
            tbody.innerHTML = '';
            records.forEach(record => {
                const statusBadge = record.status === 'completed' ? 
                    '<span class="badge bg-success">Успешно</span>' : 
                    '<span class="badge bg-danger">Ошибка</span>';
                const processedAt = record.processed_at ? 
                    new Date(record.processed_at).toLocaleString() : '-';
                const row = document.createElement('tr');
                row.innerHTML = '<td>' + record.id + '</td>' +
                    '<td>' + escapeHtml(record.original_name || '-') + '</td>' +
                    '<td>' + escapeHtml(record.normalized_name || '-') + '</td>' +
                    '<td>' + escapeHtml(record.kpved_code || '-') + (record.kpved_name ? '<br><small class="text-muted">' + escapeHtml(record.kpved_name) + '</small>' : '') + '</td>' +
                    '<td>' + statusBadge + '</td>' +
                    '<td>' + processedAt + '</td>';
                tbody.appendChild(row);
            });
        }
        function escapeHtml(text) {
            const div = document.createElement('div');
            div.textContent = text;
            return div.innerHTML;
        }
        function updateUI(data) {
            document.getElementById('totalRecords').textContent = data.db_stats.total.toLocaleString();
            document.getElementById('processedRecords').textContent = data.db_stats.completed.toLocaleString();
            document.getElementById('pendingRecords').textContent = data.db_stats.pending.toLocaleString();
            document.getElementById('errorRecords').textContent = data.db_stats.errors.toLocaleString();
            processingActive = data.processing;
            if (processingActive) {
                document.getElementById('processingBadge').classList.remove('d-none');
                document.getElementById('idleBadge').classList.add('d-none');
                document.getElementById('startBtn').disabled = true;
                document.getElementById('startProcessingBtn').disabled = true;
                document.getElementById('stopBtn').disabled = false;
            } else {
                document.getElementById('processingBadge').classList.add('d-none');
                document.getElementById('idleBadge').classList.remove('d-none');
                document.getElementById('startBtn').disabled = false;
                document.getElementById('startProcessingBtn').disabled = false;
                document.getElementById('stopBtn').disabled = true;
            }
            if (data.current_stats) {
                const stats = data.current_stats;
                const progressPercent = stats.total > 0 ? Math.floor((stats.processed / stats.total) * 100) : 0;
                document.getElementById('progressPercent').textContent = progressPercent + '%';
                document.getElementById('progressBar').style.width = progressPercent + '%';
                document.getElementById('progressText').innerHTML = 'Обработано <strong>' + stats.processed.toLocaleString() + '</strong> из <strong>' + stats.total.toLocaleString() + '</strong> записей';
                document.getElementById('startTime').textContent = new Date(stats.start_time).toLocaleString();
                const elapsed = Math.floor((Date.now() - new Date(stats.start_time).getTime()) / 1000);
                document.getElementById('elapsedTime').textContent = formatTime(elapsed);
                if (progressPercent < 100 && stats.processed > 0) {
                    const remaining = Math.floor((elapsed * (stats.total - stats.processed)) / stats.processed);
                    document.getElementById('remainingTime').textContent = formatTime(remaining);
                } else {
                    document.getElementById('remainingTime').textContent = '-';
                }
                const speed = stats.processed > 0 ? Math.floor((stats.processed / elapsed) * 60) : 0;
                document.getElementById('processingSpeed').textContent = speed > 0 ? speed + ' записей/мин' : '0 записей/мин';
                updateProgressChart(progressPercent);
                const threadActive = processingActive;
                const maxWorkers = stats.max_workers || 2;
                // Обновляем статус потоков в зависимости от max_workers
                for (let i = 1; i <= 2; i++) {
                    const indicator = document.getElementById('thread' + i + 'Indicator');
                    const status = document.getElementById('thread' + i + 'Status');
                    if (i <= maxWorkers) {
                        if (threadActive) {
                            indicator.classList.add('thread-active');
                            indicator.classList.remove('thread-idle');
                            status.textContent = 'Активен';
                        } else {
                            indicator.classList.remove('thread-active');
                            indicator.classList.add('thread-idle');
                            status.textContent = 'Ожидание';
                        }
                    } else {
                        indicator.classList.remove('thread-active');
                        indicator.classList.add('thread-idle');
                        status.textContent = 'Не используется';
                    }
                }
            } else {
                document.getElementById('progressPercent').textContent = '0%';
                document.getElementById('progressBar').style.width = '0%';
                document.getElementById('progressText').innerHTML = 'Обработано <strong>0</strong> из <strong>0</strong> записей';
                document.getElementById('startTime').textContent = '-';
                document.getElementById('elapsedTime').textContent = '-';
                document.getElementById('remainingTime').textContent = '-';
                document.getElementById('processingSpeed').textContent = '0 записей/мин';
            }
            document.getElementById('lastUpdateTime').textContent = new Date().toLocaleTimeString();
        }
        function updateProgressChart(progress) {
            const now = new Date();
            const timeLabel = now.getHours() + ':' + now.getMinutes().toString().padStart(2, '0');
            progressData.labels.push(timeLabel);
            progressData.values.push(progress);
            if (progressData.labels.length > 10) {
                progressData.labels.shift();
                progressData.values.shift();
            }
            progressChart.update();
        }
        function startProcessing() {
            if (processingActive) {
                showWarning('Обработка уже запущена');
                return;
            }
            if (!confirm('Запустить обработку необработанных номенклатур?')) {
                return;
            }
            const startBtn = document.getElementById('startBtn');
            const startProcessingBtn = document.getElementById('startProcessingBtn');
            const originalText = startBtn.innerHTML;
            const originalText2 = startProcessingBtn.innerHTML;
            startBtn.disabled = true;
            startProcessingBtn.disabled = true;
            startBtn.innerHTML = '<i class="fas fa-spinner fa-spin me-2"></i>Запуск...';
            startProcessingBtn.innerHTML = '<i class="fas fa-spinner fa-spin me-1"></i>Запуск...';
            fetch('/api/nomenclature/process', { method: 'POST', headers: { 'Content-Type': 'application/json' } })
                .then(response => {
                    if (response.ok) {
                        showSuccess('Обработка запущена');
                        setTimeout(() => loadData(), 1000);
                    } else {
                        throw new Error('Ошибка запуска обработки');
                    }
                })
                .catch(error => {
                    console.error('Ошибка:', error);
                    showError('Не удалось запустить обработку');
                    startBtn.disabled = false;
                    startProcessingBtn.disabled = false;
                    startBtn.innerHTML = originalText;
                    startProcessingBtn.innerHTML = originalText2;
                });
        }
        function stopProcessing() {
            showWarning('Функция остановки обработки будет реализована в будущем');
        }
        function toggleTheme() {
            document.body.classList.toggle('dark-mode');
            const themeIcon = document.querySelector('#themeToggle i');
            if (document.body.classList.contains('dark-mode')) {
                themeIcon.classList.remove('fa-moon');
                themeIcon.classList.add('fa-sun');
            } else {
                themeIcon.classList.remove('fa-sun');
                themeIcon.classList.add('fa-moon');
            }
        }
        function startAutoRefresh() {
            refreshInterval = setInterval(() => {
                if (document.getElementById('autoRefresh').checked) { loadData(); }
            }, 5000);
        }
        function toggleAutoRefresh() {
            if (document.getElementById('autoRefresh').checked) { startAutoRefresh(); } else { clearInterval(refreshInterval); }
        }
        function formatTime(seconds) {
            const hours = Math.floor(seconds / 3600);
            const minutes = Math.floor((seconds % 3600) / 60);
            if (hours > 0) { return hours + 'ч ' + minutes + 'м'; } else { return minutes + 'м'; }
        }
        function showSuccess(message) { console.log('Успех:', message); }
        function showError(message) { console.error('Ошибка:', message); }
        function showWarning(message) { console.warn('Предупреждение:', message); }
    </script>
</body>
</html>`
}

// handleClients обрабатывает запросы к /api/clients
func (s *Server) handleClients(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleGetClients(w, r)
	case http.MethodPost:
		s.handleCreateClient(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleClientRoutes обрабатывает запросы к /api/clients/{id} и вложенным маршрутам
func (s *Server) handleClientRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/clients/")
	parts := strings.Split(path, "/")

	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "Client ID required", http.StatusBadRequest)
		return
	}

	clientID, err := strconv.Atoi(parts[0])
	if err != nil {
		http.Error(w, "Invalid client ID", http.StatusBadRequest)
		return
	}

	// Обработка вложенных маршрутов
	if len(parts) > 1 {
		switch parts[1] {
		case "projects":
			if len(parts) == 2 {
				// GET/POST /api/clients/{id}/projects
				if r.Method == http.MethodGet {
					s.handleGetClientProjects(w, r, clientID)
				} else if r.Method == http.MethodPost {
					s.handleCreateClientProject(w, r, clientID)
				} else {
					http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
				}
				return
			}
			// Обработка /api/clients/{id}/projects/{projectId}...
			if len(parts) >= 3 {
				projectID, err := strconv.Atoi(parts[2])
				if err != nil {
					http.Error(w, "Invalid project ID", http.StatusBadRequest)
					return
				}

				if len(parts) == 3 {
					// GET/PUT/DELETE /api/clients/{id}/projects/{projectId}
					switch r.Method {
					case http.MethodGet:
						s.handleGetClientProject(w, r, clientID, projectID)
					case http.MethodPut:
						s.handleUpdateClientProject(w, r, clientID, projectID)
					case http.MethodDelete:
						s.handleDeleteClientProject(w, r, clientID, projectID)
					default:
						http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
					}
					return
				}

				if len(parts) == 4 && parts[3] == "benchmarks" {
					// GET/POST /api/clients/{id}/projects/{projectId}/benchmarks
					if r.Method == http.MethodGet {
						s.handleGetClientBenchmarks(w, r, clientID, projectID)
					} else if r.Method == http.MethodPost {
						s.handleCreateClientBenchmark(w, r, clientID, projectID)
					} else {
						http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
					}
					return
				}

				// Обработка /api/clients/{id}/projects/{projectId}/databases
				if parts[3] == "databases" {
					if len(parts) == 4 {
						// GET/POST /api/clients/{id}/projects/{projectId}/databases
						if r.Method == http.MethodGet {
							s.handleGetProjectDatabases(w, r, clientID, projectID)
						} else if r.Method == http.MethodPost {
							s.handleCreateProjectDatabase(w, r, clientID, projectID)
						} else {
							http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
						}
						return
					}

					if len(parts) == 5 {
						// GET/PUT/DELETE /api/clients/{id}/projects/{projectId}/databases/{dbId}
						dbID, err := strconv.Atoi(parts[4])
						if err != nil {
							http.Error(w, "Invalid database ID", http.StatusBadRequest)
							return
						}

						switch r.Method {
						case http.MethodGet:
							s.handleGetProjectDatabase(w, r, clientID, projectID, dbID)
						case http.MethodPut:
							s.handleUpdateProjectDatabase(w, r, clientID, projectID, dbID)
						case http.MethodDelete:
							s.handleDeleteProjectDatabase(w, r, clientID, projectID, dbID)
						default:
							http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
						}
						return
					}
				}

				if len(parts) == 4 && parts[3] == "normalization" {
					// Обработка /api/clients/{id}/projects/{projectId}/normalization/...
					if len(parts) == 5 {
						switch parts[4] {
						case "start":
							if r.Method == http.MethodPost {
								s.handleStartClientNormalization(w, r, clientID, projectID)
							} else {
								http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
							}
							return
						case "stop":
							if r.Method == http.MethodPost {
								s.handleStopClientNormalization(w, r, clientID, projectID)
							} else {
								http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
							}
							return
						case "status":
							if r.Method == http.MethodGet {
								s.handleGetClientNormalizationStatus(w, r, clientID, projectID)
							} else {
								http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
							}
							return
						case "stats":
							if r.Method == http.MethodGet {
								s.handleGetClientNormalizationStats(w, r, clientID, projectID)
							} else {
								http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
							}
							return
						}
					}
					http.Error(w, "Invalid route", http.StatusNotFound)
					return
				}
			}
		}
		http.Error(w, "Invalid route", http.StatusNotFound)
		return
	}

	// Обработка /api/clients/{id}
	switch r.Method {
	case http.MethodGet:
		s.handleGetClient(w, r, clientID)
	case http.MethodPut:
		s.handleUpdateClient(w, r, clientID)
	case http.MethodDelete:
		s.handleDeleteClient(w, r, clientID)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleGetClients получает список клиентов
func (s *Server) handleGetClients(w http.ResponseWriter, r *http.Request) {
	clients, err := s.serviceDB.GetClientsWithStats()
	if err != nil {
		s.writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.writeJSONResponse(w, clients, http.StatusOK)
}

// handleCreateClient создает нового клиента
func (s *Server) handleCreateClient(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name         string `json:"name"`
		LegalName    string `json:"legal_name"`
		Description  string `json:"description"`
		ContactEmail string `json:"contact_email"`
		ContactPhone string `json:"contact_phone"`
		TaxID        string `json:"tax_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		s.writeJSONError(w, "Name is required", http.StatusBadRequest)
		return
	}

	client, err := s.serviceDB.CreateClient(req.Name, req.LegalName, req.Description, req.ContactEmail, req.ContactPhone, req.TaxID, "system")
	if err != nil {
		s.writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.writeJSONResponse(w, client, http.StatusCreated)
}

// handleGetClient получает клиента по ID
func (s *Server) handleGetClient(w http.ResponseWriter, r *http.Request, clientID int) {
	client, err := s.serviceDB.GetClient(clientID)
	if err != nil {
		s.writeJSONError(w, "Client not found", http.StatusNotFound)
		return
	}

	projects, err := s.serviceDB.GetClientProjects(clientID)
	if err != nil {
		s.writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Подсчет статистики
	var totalBenchmarks int
	var activeSessions int
	var avgQualityScore float64

	response := ClientDetailResponse{
		Client: Client{
			ID:           client.ID,
			Name:         client.Name,
			LegalName:    client.LegalName,
			Description:  client.Description,
			ContactEmail: client.ContactEmail,
			ContactPhone: client.ContactPhone,
			TaxID:        client.TaxID,
			Status:       client.Status,
			CreatedBy:    client.CreatedBy,
			CreatedAt:    client.CreatedAt,
			UpdatedAt:    client.UpdatedAt,
		},
		Projects: make([]ClientProject, len(projects)),
		Statistics: ClientStatistics{
			TotalProjects:   len(projects),
			TotalBenchmarks: totalBenchmarks,
			ActiveSessions:  activeSessions,
			AvgQualityScore: avgQualityScore,
		},
	}

	for i, p := range projects {
		response.Projects[i] = ClientProject{
			ID:                 p.ID,
			ClientID:           p.ClientID,
			Name:               p.Name,
			ProjectType:        p.ProjectType,
			Description:        p.Description,
			SourceSystem:       p.SourceSystem,
			Status:             p.Status,
			TargetQualityScore: p.TargetQualityScore,
			CreatedAt:          p.CreatedAt,
			UpdatedAt:          p.UpdatedAt,
		}
	}

	s.writeJSONResponse(w, response, http.StatusOK)
}

// handleUpdateClient обновляет клиента
func (s *Server) handleUpdateClient(w http.ResponseWriter, r *http.Request, clientID int) {
	var req struct {
		Name         string `json:"name"`
		LegalName    string `json:"legal_name"`
		Description  string `json:"description"`
		ContactEmail string `json:"contact_email"`
		ContactPhone string `json:"contact_phone"`
		TaxID        string `json:"tax_id"`
		Status       string `json:"status"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := s.serviceDB.UpdateClient(clientID, req.Name, req.LegalName, req.Description, req.ContactEmail, req.ContactPhone, req.TaxID, req.Status); err != nil {
		s.writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	client, err := s.serviceDB.GetClient(clientID)
	if err != nil {
		s.writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.writeJSONResponse(w, client, http.StatusOK)
}

// handleDeleteClient удаляет клиента
func (s *Server) handleDeleteClient(w http.ResponseWriter, r *http.Request, clientID int) {
	if err := s.serviceDB.DeleteClient(clientID); err != nil {
		s.writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.writeJSONResponse(w, map[string]string{"message": "Client deleted"}, http.StatusOK)
}

// handleGetClientProjects получает проекты клиента
func (s *Server) handleGetClientProjects(w http.ResponseWriter, r *http.Request, clientID int) {
	projects, err := s.serviceDB.GetClientProjects(clientID)
	if err != nil {
		s.writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := make([]ClientProject, len(projects))
	for i, p := range projects {
		response[i] = ClientProject{
			ID:                 p.ID,
			ClientID:           p.ClientID,
			Name:               p.Name,
			ProjectType:        p.ProjectType,
			Description:        p.Description,
			SourceSystem:       p.SourceSystem,
			Status:             p.Status,
			TargetQualityScore: p.TargetQualityScore,
			CreatedAt:          p.CreatedAt,
			UpdatedAt:          p.UpdatedAt,
		}
	}

	s.writeJSONResponse(w, response, http.StatusOK)
}

// handleCreateClientProject создает проект клиента
func (s *Server) handleCreateClientProject(w http.ResponseWriter, r *http.Request, clientID int) {
	var req struct {
		Name               string  `json:"name"`
		ProjectType        string  `json:"project_type"`
		Description        string  `json:"description"`
		SourceSystem       string  `json:"source_system"`
		TargetQualityScore float64 `json:"target_quality_score"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" || req.ProjectType == "" {
		s.writeJSONError(w, "Name and project_type are required", http.StatusBadRequest)
		return
	}

	project, err := s.serviceDB.CreateClientProject(clientID, req.Name, req.ProjectType, req.Description, req.SourceSystem, req.TargetQualityScore)
	if err != nil {
		s.writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := ClientProject{
		ID:                 project.ID,
		ClientID:           project.ClientID,
		Name:               project.Name,
		ProjectType:        project.ProjectType,
		Description:        project.Description,
		SourceSystem:       project.SourceSystem,
		Status:             project.Status,
		TargetQualityScore: project.TargetQualityScore,
		CreatedAt:          project.CreatedAt,
		UpdatedAt:          project.UpdatedAt,
	}

	s.writeJSONResponse(w, response, http.StatusCreated)
}

// handleGetClientProject получает проект по ID
func (s *Server) handleGetClientProject(w http.ResponseWriter, r *http.Request, clientID, projectID int) {
	project, err := s.serviceDB.GetClientProject(projectID)
	if err != nil {
		s.writeJSONError(w, "Project not found", http.StatusNotFound)
		return
	}

	if project.ClientID != clientID {
		s.writeJSONError(w, "Project does not belong to this client", http.StatusBadRequest)
		return
	}

	// Получаем эталоны проекта
	benchmarks, err := s.serviceDB.GetClientBenchmarks(projectID, "", false)
	if err != nil {
		log.Printf("Error fetching benchmarks: %v", err)
		benchmarks = []*database.ClientBenchmark{}
	}

	// Вычисляем статистику
	totalBenchmarks := len(benchmarks)
	approvedBenchmarks := 0
	totalQuality := 0.0
	for _, b := range benchmarks {
		if b.IsApproved {
			approvedBenchmarks++
		}
		totalQuality += b.QualityScore
	}
	avgQuality := 0.0
	if totalBenchmarks > 0 {
		avgQuality = totalQuality / float64(totalBenchmarks)
	}

	response := map[string]interface{}{
		"project": ClientProject{
			ID:                 project.ID,
			ClientID:           project.ClientID,
			Name:               project.Name,
			ProjectType:        project.ProjectType,
			Description:        project.Description,
			SourceSystem:       project.SourceSystem,
			Status:             project.Status,
			TargetQualityScore: project.TargetQualityScore,
			CreatedAt:          project.CreatedAt,
			UpdatedAt:          project.UpdatedAt,
		},
		"benchmarks": benchmarks[:min(10, len(benchmarks))], // Первые 10 эталонов
		"statistics": map[string]interface{}{
			"total_benchmarks":    totalBenchmarks,
			"approved_benchmarks": approvedBenchmarks,
			"avg_quality_score":   avgQuality,
		},
	}

	s.writeJSONResponse(w, response, http.StatusOK)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// handleUpdateClientProject обновляет проект клиента
func (s *Server) handleUpdateClientProject(w http.ResponseWriter, r *http.Request, clientID, projectID int) {
	project, err := s.serviceDB.GetClientProject(projectID)
	if err != nil {
		s.writeJSONError(w, "Project not found", http.StatusNotFound)
		return
	}

	if project.ClientID != clientID {
		s.writeJSONError(w, "Project does not belong to this client", http.StatusBadRequest)
		return
	}

	var req struct {
		Name               string  `json:"name"`
		ProjectType        string  `json:"project_type"`
		Description        string  `json:"description"`
		SourceSystem       string  `json:"source_system"`
		Status             string  `json:"status"`
		TargetQualityScore float64 `json:"target_quality_score"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if err := s.serviceDB.UpdateClientProject(projectID, req.Name, req.ProjectType, req.Description, req.SourceSystem, req.Status, req.TargetQualityScore); err != nil {
		s.writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Получаем обновленный проект
	updated, err := s.serviceDB.GetClientProject(projectID)
	if err != nil {
		s.writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := ClientProject{
		ID:                 updated.ID,
		ClientID:           updated.ClientID,
		Name:               updated.Name,
		ProjectType:        updated.ProjectType,
		Description:        updated.Description,
		SourceSystem:       updated.SourceSystem,
		Status:             updated.Status,
		TargetQualityScore: updated.TargetQualityScore,
		CreatedAt:          updated.CreatedAt,
		UpdatedAt:          updated.UpdatedAt,
	}

	s.writeJSONResponse(w, response, http.StatusOK)
}

// handleDeleteClientProject удаляет проект клиента
func (s *Server) handleDeleteClientProject(w http.ResponseWriter, r *http.Request, clientID, projectID int) {
	project, err := s.serviceDB.GetClientProject(projectID)
	if err != nil {
		s.writeJSONError(w, "Project not found", http.StatusNotFound)
		return
	}

	if project.ClientID != clientID {
		s.writeJSONError(w, "Project does not belong to this client", http.StatusBadRequest)
		return
	}

	if err := s.serviceDB.DeleteClientProject(projectID); err != nil {
		s.writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.writeJSONResponse(w, map[string]string{"message": "Project deleted"}, http.StatusOK)
}

// handleGetClientBenchmarks получает эталоны проекта
func (s *Server) handleGetClientBenchmarks(w http.ResponseWriter, r *http.Request, clientID, projectID int) {
	project, err := s.serviceDB.GetClientProject(projectID)
	if err != nil {
		s.writeJSONError(w, "Project not found", http.StatusNotFound)
		return
	}

	if project.ClientID != clientID {
		s.writeJSONError(w, "Project does not belong to this client", http.StatusBadRequest)
		return
	}

	category := r.URL.Query().Get("category")
	approvedOnly := r.URL.Query().Get("approved_only") == "true"

	benchmarks, err := s.serviceDB.GetClientBenchmarks(projectID, category, approvedOnly)
	if err != nil {
		s.writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	responseBenchmarks := make([]ClientBenchmark, len(benchmarks))
	for i, b := range benchmarks {
		responseBenchmarks[i] = ClientBenchmark{
			ID:              b.ID,
			ClientProjectID: b.ClientProjectID,
			OriginalName:    b.OriginalName,
			NormalizedName:  b.NormalizedName,
			Category:        b.Category,
			Subcategory:     b.Subcategory,
			Attributes:      b.Attributes,
			QualityScore:    b.QualityScore,
			IsApproved:      b.IsApproved,
			ApprovedBy:      b.ApprovedBy,
			ApprovedAt:      b.ApprovedAt,
			SourceDatabase:  b.SourceDatabase,
			UsageCount:      b.UsageCount,
			CreatedAt:       b.CreatedAt,
			UpdatedAt:       b.UpdatedAt,
		}
	}

	s.writeJSONResponse(w, map[string]interface{}{
		"benchmarks": responseBenchmarks,
		"total":      len(responseBenchmarks),
	}, http.StatusOK)
}

// handleCreateClientBenchmark создает эталон
func (s *Server) handleCreateClientBenchmark(w http.ResponseWriter, r *http.Request, clientID, projectID int) {
	var req struct {
		OriginalName   string  `json:"original_name"`
		NormalizedName string  `json:"normalized_name"`
		Category       string  `json:"category"`
		Subcategory    string  `json:"subcategory"`
		Attributes     string  `json:"attributes"`
		QualityScore   float64 `json:"quality_score"`
		SourceDatabase string  `json:"source_database"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.OriginalName == "" || req.NormalizedName == "" || req.Category == "" {
		s.writeJSONError(w, "original_name, normalized_name and category are required", http.StatusBadRequest)
		return
	}

	benchmark, err := s.serviceDB.CreateClientBenchmark(projectID, req.OriginalName, req.NormalizedName, req.Category, req.Subcategory, req.Attributes, req.SourceDatabase, req.QualityScore)
	if err != nil {
		s.writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	response := ClientBenchmark{
		ID:              benchmark.ID,
		ClientProjectID: benchmark.ClientProjectID,
		OriginalName:    benchmark.OriginalName,
		NormalizedName:  benchmark.NormalizedName,
		Category:        benchmark.Category,
		Subcategory:     benchmark.Subcategory,
		Attributes:      benchmark.Attributes,
		QualityScore:    benchmark.QualityScore,
		IsApproved:      benchmark.IsApproved,
		ApprovedBy:      benchmark.ApprovedBy,
		ApprovedAt:      benchmark.ApprovedAt,
		SourceDatabase:  benchmark.SourceDatabase,
		UsageCount:      benchmark.UsageCount,
		CreatedAt:       benchmark.CreatedAt,
		UpdatedAt:       benchmark.UpdatedAt,
	}

	s.writeJSONResponse(w, response, http.StatusCreated)
}

// handleGetProjectDatabases получает список баз данных проекта
func (s *Server) handleGetProjectDatabases(w http.ResponseWriter, r *http.Request, clientID, projectID int) {
	project, err := s.serviceDB.GetClientProject(projectID)
	if err != nil {
		s.writeJSONError(w, "Project not found", http.StatusNotFound)
		return
	}

	if project.ClientID != clientID {
		s.writeJSONError(w, "Project does not belong to this client", http.StatusBadRequest)
		return
	}

	activeOnly := r.URL.Query().Get("active_only") == "true"

	databases, err := s.serviceDB.GetProjectDatabases(projectID, activeOnly)
	if err != nil {
		s.writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.writeJSONResponse(w, map[string]interface{}{
		"databases": databases,
		"total":     len(databases),
	}, http.StatusOK)
}

// handleCreateProjectDatabase создает новую базу данных для проекта
func (s *Server) handleCreateProjectDatabase(w http.ResponseWriter, r *http.Request, clientID, projectID int) {
	var req struct {
		Name        string `json:"name"`
		FilePath    string `json:"file_path"`
		Description string `json:"description"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" || req.FilePath == "" {
		s.writeJSONError(w, "Name and file_path are required", http.StatusBadRequest)
		return
	}

	project, err := s.serviceDB.GetClientProject(projectID)
	if err != nil {
		s.writeJSONError(w, "Project not found", http.StatusNotFound)
		return
	}

	if project.ClientID != clientID {
		s.writeJSONError(w, "Project does not belong to this client", http.StatusBadRequest)
		return
	}

	// Проверяем, не существует ли уже база данных с таким именем для этого проекта
	existingDatabases, err := s.serviceDB.GetProjectDatabases(projectID, false)
	if err == nil {
		for _, existingDB := range existingDatabases {
			// Проверяем по имени или по пути к файлу
			if existingDB.Name == req.Name || (req.FilePath != "" && existingDB.FilePath == req.FilePath) {
				// Возвращаем существующую базу данных вместо создания дубликата
				s.writeJSONResponse(w, existingDB, http.StatusOK)
				return
			}
		}
	}

	// Получаем размер файла если файл существует
	var fileSize int64 = 0
	if fileInfo, err := os.Stat(req.FilePath); err == nil {
		fileSize = fileInfo.Size()
	}

	database, err := s.serviceDB.CreateProjectDatabase(projectID, req.Name, req.FilePath, req.Description, fileSize)
	if err != nil {
		s.writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.writeJSONResponse(w, database, http.StatusCreated)
}

// handleGetProjectDatabase получает базу данных проекта
func (s *Server) handleGetProjectDatabase(w http.ResponseWriter, r *http.Request, clientID, projectID, dbID int) {
	project, err := s.serviceDB.GetClientProject(projectID)
	if err != nil {
		s.writeJSONError(w, "Project not found", http.StatusNotFound)
		return
	}

	if project.ClientID != clientID {
		s.writeJSONError(w, "Project does not belong to this client", http.StatusBadRequest)
		return
	}

	database, err := s.serviceDB.GetProjectDatabase(dbID)
	if err != nil {
		s.writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if database == nil {
		s.writeJSONError(w, "Database not found", http.StatusNotFound)
		return
	}

	if database.ClientProjectID != projectID {
		s.writeJSONError(w, "Database does not belong to this project", http.StatusBadRequest)
		return
	}

	s.writeJSONResponse(w, database, http.StatusOK)
}

// handleUpdateProjectDatabase обновляет базу данных проекта
func (s *Server) handleUpdateProjectDatabase(w http.ResponseWriter, r *http.Request, clientID, projectID, dbID int) {
	var req struct {
		Name        string `json:"name"`
		FilePath    string `json:"file_path"`
		Description string `json:"description"`
		IsActive    bool   `json:"is_active"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	project, err := s.serviceDB.GetClientProject(projectID)
	if err != nil {
		s.writeJSONError(w, "Project not found", http.StatusNotFound)
		return
	}

	if project.ClientID != clientID {
		s.writeJSONError(w, "Project does not belong to this client", http.StatusBadRequest)
		return
	}

	database, err := s.serviceDB.GetProjectDatabase(dbID)
	if err != nil {
		s.writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if database == nil {
		s.writeJSONError(w, "Database not found", http.StatusNotFound)
		return
	}

	if database.ClientProjectID != projectID {
		s.writeJSONError(w, "Database does not belong to this project", http.StatusBadRequest)
		return
	}

	err = s.serviceDB.UpdateProjectDatabase(dbID, req.Name, req.FilePath, req.Description, req.IsActive)
	if err != nil {
		s.writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	updatedDatabase, err := s.serviceDB.GetProjectDatabase(dbID)
	if err != nil {
		s.writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.writeJSONResponse(w, updatedDatabase, http.StatusOK)
}

// handleDeleteProjectDatabase удаляет базу данных проекта
func (s *Server) handleDeleteProjectDatabase(w http.ResponseWriter, r *http.Request, clientID, projectID, dbID int) {
	project, err := s.serviceDB.GetClientProject(projectID)
	if err != nil {
		s.writeJSONError(w, "Project not found", http.StatusNotFound)
		return
	}

	if project.ClientID != clientID {
		s.writeJSONError(w, "Project does not belong to this client", http.StatusBadRequest)
		return
	}

	database, err := s.serviceDB.GetProjectDatabase(dbID)
	if err != nil {
		s.writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if database == nil {
		s.writeJSONError(w, "Database not found", http.StatusNotFound)
		return
	}

	if database.ClientProjectID != projectID {
		s.writeJSONError(w, "Database does not belong to this project", http.StatusBadRequest)
		return
	}

	err = s.serviceDB.DeleteProjectDatabase(dbID)
	if err != nil {
		s.writeJSONError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.writeJSONResponse(w, map[string]string{
		"message": "Database deleted successfully",
	}, http.StatusOK)
}

// handleStartClientNormalization запускает нормализацию для клиента
func (s *Server) handleStartClientNormalization(w http.ResponseWriter, r *http.Request, clientID, projectID int) {
	s.normalizerMutex.Lock()
	defer s.normalizerMutex.Unlock()

	if s.normalizerRunning {
		s.writeJSONError(w, "Normalization is already running", http.StatusBadRequest)
		return
	}

	// Читаем путь к базе данных из запроса
	var req struct {
		DatabasePath string `json:"database_path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.DatabasePath == "" {
		s.writeJSONError(w, "Database path is required", http.StatusBadRequest)
		return
	}

	// Проверяем существование проекта
	project, err := s.serviceDB.GetClientProject(projectID)
	if err != nil {
		s.writeJSONError(w, "Project not found", http.StatusNotFound)
		return
	}

	if project.ClientID != clientID {
		s.writeJSONError(w, "Project does not belong to this client", http.StatusBadRequest)
		return
	}

	// Открываем подключение к указанной базе данных
	sourceDB, err := database.NewDB(req.DatabasePath)
	if err != nil {
		s.writeJSONError(w, fmt.Sprintf("Failed to open database: %v", err), http.StatusBadRequest)
		return
	}

	// Получаем все записи из catalog_items указанной БД
	items, err := sourceDB.GetAllCatalogItems()
	if err != nil {
		sourceDB.Close()
		s.writeJSONError(w, fmt.Sprintf("Failed to read data: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("Starting normalization for project %d with %d items from %s", projectID, len(items), req.DatabasePath)

	// Создаем клиентский нормализатор (используем sourceDB для чтения данных)
	// Передаем workerConfigManager для получения правильной модели
	clientNormalizer := normalization.NewClientNormalizerWithConfig(clientID, projectID, sourceDB, s.serviceDB, s.normalizerEvents, s.workerConfigManager)

	// Запускаем нормализацию в отдельной горутине
	s.normalizerRunning = true
	go func() {
		defer func() {
			s.normalizerMutex.Lock()
			s.normalizerRunning = false
			s.normalizerMutex.Unlock()
			sourceDB.Close() // Закрываем БД после завершения
			log.Printf("Normalization completed for project %d", projectID)
		}()

		_, err := clientNormalizer.ProcessWithClientBenchmarks(items)
		if err != nil {
			select {
			case s.normalizerEvents <- fmt.Sprintf("Ошибка нормализации: %v", err):
			default:
			}
			log.Printf("Ошибка клиентской нормализации: %v", err)
		}
	}()

	s.writeJSONResponse(w, map[string]interface{}{
		"status":        "started",
		"message":       "Normalization started",
		"database_path": req.DatabasePath,
		"items_count":   len(items),
	}, http.StatusOK)
}

// handleStopClientNormalization останавливает нормализацию для клиента
func (s *Server) handleStopClientNormalization(w http.ResponseWriter, r *http.Request, clientID, projectID int) {
	s.normalizerMutex.Lock()
	defer s.normalizerMutex.Unlock()

	if !s.normalizerRunning {
		s.writeJSONError(w, "Normalization is not running", http.StatusBadRequest)
		return
	}

	// Останавливаем нормализацию
	s.normalizerRunning = false

	s.writeJSONResponse(w, map[string]interface{}{
		"status":  "stopped",
		"message": "Normalization stopped",
	}, http.StatusOK)
}

// handleGetClientNormalizationStatus получает статус нормализации для клиента
func (s *Server) handleGetClientNormalizationStatus(w http.ResponseWriter, r *http.Request, clientID, projectID int) {
	s.normalizerMutex.RLock()
	isRunning := s.normalizerRunning
	s.normalizerMutex.RUnlock()

	response := map[string]interface{}{
		"is_running": isRunning,
		"client_id":  clientID,
		"project_id": projectID,
	}

	s.writeJSONResponse(w, response, http.StatusOK)
}

// handleGetClientNormalizationStats получает статистику нормализации для клиента
func (s *Server) handleGetClientNormalizationStats(w http.ResponseWriter, r *http.Request, clientID, projectID int) {
	s.normalizerMutex.RLock()
	isRunning := s.normalizerRunning
	s.normalizerMutex.RUnlock()

	// Получаем статистику из БД (упрощенная версия)
	stats := map[string]interface{}{
		"is_running":        isRunning,
		"total_processed":   0,
		"total_groups":      0,
		"benchmark_matches": 0,
		"ai_enhanced":       0,
		"basic_normalized":  0,
	}

	s.writeJSONResponse(w, stats, http.StatusOK)
}

// handleQualityStats возвращает статистику качества нормализации
func (s *Server) handleQualityStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Используем db, так как записи сохраняются туда (консистентно с handleMonitoringMetrics)
	stats, err := s.db.GetQualityStats()
	if err != nil {
		log.Printf("Error getting quality stats: %v", err)
		s.writeJSONError(w, fmt.Sprintf("Failed to get quality stats: %v", err), http.StatusInternalServerError)
		return
	}

	s.writeJSONResponse(w, stats, http.StatusOK)
}

// handleMonitoringMetrics возвращает общую статистику производительности
func (s *Server) handleMonitoringMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Получаем реальную статистику от нормализатора
	var statsCollector *normalization.StatsCollector
	var cacheStats normalization.CacheStats
	hasCacheStats := false

	if s.normalizer != nil && s.normalizer.GetAINormalizer() != nil {
		statsCollector = s.normalizer.GetAINormalizer().GetStatsCollector()
		cacheStats = s.normalizer.GetAINormalizer().GetCacheStats()
		hasCacheStats = true
	}

	// Получаем статистику качества из БД (используем db, так как записи сохраняются туда)
	qualityStatsMap, err := s.db.GetQualityStats()
	if err != nil {
		log.Printf("Error getting quality stats: %v", err)
		qualityStatsMap = make(map[string]interface{})
	}

	// Извлекаем значения из карты БД (используем как fallback)
	dbTotalNormalized := int64(0)
	dbBasicNormalized := int64(0)
	dbAIEnhanced := int64(0)
	dbBenchmarkQuality := int64(0)
	dbAverageQualityScore := 0.0

	if total, ok := qualityStatsMap["total_items"].(int); ok {
		dbTotalNormalized = int64(total)
	}
	if avg, ok := qualityStatsMap["average_quality"].(float64); ok {
		dbAverageQualityScore = avg
	}
	if benchmark, ok := qualityStatsMap["benchmark_count"].(int); ok {
		dbBenchmarkQuality = int64(benchmark)
	}
	if byLevel, ok := qualityStatsMap["by_level"].(map[string]map[string]interface{}); ok {
		if basicStats, ok := byLevel["basic"]; ok {
			if count, ok := basicStats["count"].(int); ok {
				dbBasicNormalized = int64(count)
			}
		}
		if aiStats, ok := byLevel["ai_enhanced"]; ok {
			if count, ok := aiStats["count"].(int); ok {
				dbAIEnhanced = int64(count)
			}
		}
	}

	// Используем метрики из StatsCollector как основной источник, БД как fallback
	totalNormalized := dbTotalNormalized
	basicNormalized := dbBasicNormalized
	aiEnhanced := dbAIEnhanced
	benchmarkQuality := dbBenchmarkQuality
	averageQualityScore := dbAverageQualityScore

	if statsCollector != nil {
		perfMetrics := statsCollector.GetMetrics()
		// Используем метрики из StatsCollector если они доступны
		if perfMetrics.TotalNormalized > 0 {
			totalNormalized = perfMetrics.TotalNormalized
			basicNormalized = perfMetrics.BasicNormalized
			aiEnhanced = perfMetrics.AIEnhanced
			benchmarkQuality = perfMetrics.BenchmarkQuality
			if perfMetrics.AverageQualityScore > 0 {
				averageQualityScore = perfMetrics.AverageQualityScore
			} else if dbAverageQualityScore > 0 {
				averageQualityScore = dbAverageQualityScore
			}
		}
	}

	// Рассчитываем uptime
	uptime := time.Since(s.startTime).Seconds()

	// Рассчитываем throughput (за всё время работы)
	throughput := 0.0
	if uptime > 0 && totalNormalized > 0 {
		throughput = float64(totalNormalized) / uptime
	}

	// Формируем ответ
	summary := map[string]interface{}{
		"uptime_seconds":              uptime,
		"throughput_items_per_second": throughput,
		"ai": map[string]interface{}{
			"total_requests":     0,
			"successful":         0,
			"failed":             0,
			"success_rate":       0.0,
			"average_latency_ms": 0.0,
		},
		"cache": map[string]interface{}{
			"hits":            0,
			"misses":          0,
			"hit_rate":        0.0,
			"size":            0,
			"memory_usage_kb": 0.0,
		},
		"quality": map[string]interface{}{
			"total_normalized":      totalNormalized,
			"basic":                 basicNormalized,
			"ai_enhanced":           aiEnhanced,
			"benchmark":             benchmarkQuality,
			"average_quality_score": averageQualityScore,
		},
	}

	// Добавляем реальные AI метрики если доступны
	if statsCollector != nil {
		perfMetrics := statsCollector.GetMetrics()
		successRate := 0.0
		if perfMetrics.TotalAIRequests > 0 {
			successRate = float64(perfMetrics.SuccessfulAIRequest) / float64(perfMetrics.TotalAIRequests)
		}
		avgLatencyMs := float64(perfMetrics.AverageAILatency.Milliseconds())

		summary["ai"] = map[string]interface{}{
			"total_requests":     perfMetrics.TotalAIRequests,
			"successful":         perfMetrics.SuccessfulAIRequest,
			"failed":             perfMetrics.FailedAIRequests,
			"success_rate":       successRate,
			"average_latency_ms": avgLatencyMs,
		}
	}

	// Добавляем реальные cache метрики если доступны
	if hasCacheStats {
		summary["cache"] = map[string]interface{}{
			"hits":            cacheStats.Hits,
			"misses":          cacheStats.Misses,
			"hit_rate":        cacheStats.HitRate,
			"size":            cacheStats.Entries,
			"memory_usage_kb": float64(cacheStats.MemoryUsageB) / 1024.0,
		}
	}

	// Добавляем метрики Circuit Breaker
	summary["circuit_breaker"] = s.GetCircuitBreakerState()

	// Добавляем метрики Batch Processor
	summary["batch_processor"] = s.GetBatchProcessorStats()

	// Добавляем статус Checkpoint
	summary["checkpoint"] = s.GetCheckpointStatus()

	s.writeJSONResponse(w, summary, http.StatusOK)
}

// handleMonitoringCache возвращает статистику кеша
func (s *Server) handleMonitoringCache(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Получаем реальную статистику кеша
	var cacheStats normalization.CacheStats
	if s.normalizer != nil && s.normalizer.GetAINormalizer() != nil {
		cacheStats = s.normalizer.GetAINormalizer().GetCacheStats()
	}

	response := map[string]interface{}{
		"hits":            cacheStats.Hits,
		"misses":          cacheStats.Misses,
		"hit_rate_pct":    cacheStats.HitRate * 100.0,
		"size":            cacheStats.Entries,
		"memory_usage_kb": float64(cacheStats.MemoryUsageB) / 1024.0,
	}

	s.writeJSONResponse(w, response, http.StatusOK)
}

// handleMonitoringAI возвращает статистику AI обработки
func (s *Server) handleMonitoringAI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Получаем реальную статистику AI
	var statsCollector *normalization.StatsCollector
	var cacheStats normalization.CacheStats

	if s.normalizer != nil && s.normalizer.GetAINormalizer() != nil {
		statsCollector = s.normalizer.GetAINormalizer().GetStatsCollector()
		cacheStats = s.normalizer.GetAINormalizer().GetCacheStats()
	}

	totalCalls := int64(0)
	errors := int64(0)
	avgLatencyMs := 0.0

	if statsCollector != nil {
		perfMetrics := statsCollector.GetMetrics()
		totalCalls = perfMetrics.TotalAIRequests
		errors = perfMetrics.FailedAIRequests
		avgLatencyMs = float64(perfMetrics.AverageAILatency.Milliseconds())
	}

	cacheHitRate := 0.0
	if cacheStats.Hits+cacheStats.Misses > 0 {
		cacheHitRate = float64(cacheStats.Hits) / float64(cacheStats.Hits+cacheStats.Misses) * 100.0
	}

	stats := map[string]interface{}{
		"total_calls":    totalCalls,
		"cache_hits":     cacheStats.Hits,
		"cache_misses":   cacheStats.Misses,
		"errors":         errors,
		"avg_latency_ms": avgLatencyMs,
		"cache_hit_rate": cacheHitRate,
	}

	s.writeJSONResponse(w, stats, http.StatusOK)
}

// handleMonitoringHistory возвращает историю метрик за указанный период
func (s *Server) handleMonitoringHistory(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Парсим параметры запроса
	query := r.URL.Query()

	// Параметр from (начало периода)
	var fromTime *time.Time
	if fromStr := query.Get("from"); fromStr != "" {
		if t, err := time.Parse(time.RFC3339, fromStr); err == nil {
			fromTime = &t
		} else {
			// Попробуем другой формат
			if t, err := time.Parse("2006-01-02 15:04:05", fromStr); err == nil {
				fromTime = &t
			}
		}
	}

	// Параметр to (конец периода)
	var toTime *time.Time
	if toStr := query.Get("to"); toStr != "" {
		if t, err := time.Parse(time.RFC3339, toStr); err == nil {
			toTime = &t
		} else {
			if t, err := time.Parse("2006-01-02 15:04:05", toStr); err == nil {
				toTime = &t
			}
		}
	}

	// Параметр metricType (фильтр по типу)
	metricType := query.Get("metric_type")

	// Параметр limit (максимальное количество записей)
	limit := 100 // По умолчанию 100 записей
	if limitStr := query.Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			if l > 1000 {
				limit = 1000 // Максимум 1000 записей
			} else {
				limit = l
			}
		}
	}

	// Получаем историю метрик из БД
	snapshots, err := s.db.GetMetricsHistory(fromTime, toTime, metricType, limit)
	if err != nil {
		log.Printf("Error getting metrics history: %v", err)
		s.writeJSONError(w, fmt.Sprintf("Failed to get metrics history: %v", err), http.StatusInternalServerError)
		return
	}

	// Формируем ответ
	response := map[string]interface{}{
		"count":     len(snapshots),
		"snapshots": snapshots,
	}

	s.writeJSONResponse(w, response, http.StatusOK)
}

// handleMonitoringEvents обрабатывает SSE соединение для real-time метрик мониторинга
func (s *Server) handleMonitoringEvents(w http.ResponseWriter, r *http.Request) {
	// Устанавливаем заголовки для SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Cache-Control")

	// Проверяем поддержку Flusher
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// Отправляем начальное событие
	fmt.Fprintf(w, "data: %s\n\n", "{\"type\":\"connected\",\"message\":\"Connected to monitoring events\"}")
	flusher.Flush()

	// Создаем тикер для периодической отправки метрик
	metricsTicker := time.NewTicker(5 * time.Second)
	defer metricsTicker.Stop()

	// Heartbeat тикер
	heartbeatTicker := time.NewTicker(30 * time.Second)
	defer heartbeatTicker.Stop()

	for {
		select {
		case <-metricsTicker.C:
			// Собираем текущие метрики
			snapshot := s.CollectMetricsSnapshot()
			if snapshot != nil {
				// Формируем JSON с метриками
				metricsJSON, err := json.Marshal(map[string]interface{}{
					"type":                  "metrics",
					"timestamp":             time.Now().Format(time.RFC3339),
					"uptime_seconds":        snapshot.UptimeSeconds,
					"throughput":            snapshot.Throughput,
					"ai_success_rate":       snapshot.AISuccessRate,
					"cache_hit_rate":        snapshot.CacheHitRate,
					"batch_queue_size":      snapshot.BatchQueueSize,
					"circuit_breaker_state": snapshot.CircuitBreakerState,
					"checkpoint_progress":   snapshot.CheckpointProgress,
				})

				if err == nil {
					if _, err := fmt.Fprintf(w, "data: %s\n\n", string(metricsJSON)); err != nil {
						log.Printf("Ошибка отправки SSE метрик: %v", err)
						return
					}
					flusher.Flush()
				}
			}

		case <-heartbeatTicker.C:
			// Отправляем heartbeat для поддержания соединения
			if _, err := fmt.Fprintf(w, ": heartbeat\n\n"); err != nil {
				log.Printf("Ошибка отправки heartbeat: %v", err)
				return
			}
			flusher.Flush()

		case <-r.Context().Done():
			// Клиент отключился
			log.Printf("SSE клиент мониторинга отключился: %v", r.Context().Err())
			return
		}
	}
}

// handleNormalizationConfig обрабатывает GET/POST запросы конфигурации нормализации
func (s *Server) handleNormalizationConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// Получить текущую конфигурацию
		config, err := s.serviceDB.GetNormalizationConfig()
		if err != nil {
			log.Printf("Error getting normalization config: %v", err)
			s.writeJSONError(w, fmt.Sprintf("Failed to get config: %v", err), http.StatusInternalServerError)
			return
		}
		s.writeJSONResponse(w, config, http.StatusOK)

	case http.MethodPost:
		// Обновить конфигурацию
		var req struct {
			DatabasePath    string `json:"database_path"`
			SourceTable     string `json:"source_table"`
			ReferenceColumn string `json:"reference_column"`
			CodeColumn      string `json:"code_column"`
			NameColumn      string `json:"name_column"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			s.writeJSONError(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		// Валидация
		if req.SourceTable == "" || req.ReferenceColumn == "" || req.CodeColumn == "" || req.NameColumn == "" {
			s.writeJSONError(w, "All fields are required", http.StatusBadRequest)
			return
		}

		err := s.serviceDB.UpdateNormalizationConfig(
			req.DatabasePath,
			req.SourceTable,
			req.ReferenceColumn,
			req.CodeColumn,
			req.NameColumn,
		)
		if err != nil {
			log.Printf("Error updating normalization config: %v", err)
			s.writeJSONError(w, fmt.Sprintf("Failed to update config: %v", err), http.StatusInternalServerError)
			return
		}

		// Возвращаем обновленную конфигурацию
		config, err := s.serviceDB.GetNormalizationConfig()
		if err != nil {
			log.Printf("Error getting updated config: %v", err)
			s.writeJSONError(w, fmt.Sprintf("Config saved but failed to retrieve: %v", err), http.StatusInternalServerError)
			return
		}

		s.writeJSONResponse(w, config, http.StatusOK)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleNormalizationDatabases возвращает список доступных баз данных
func (s *Server) handleNormalizationDatabases(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Получаем список БД файлов в текущей директории
	databases := []map[string]interface{}{}

	// Добавляем текущую БД
	if s.currentDBPath != "" {
		databases = append(databases, map[string]interface{}{
			"path": s.currentDBPath,
			"name": filepath.Base(s.currentDBPath),
			"type": "current",
		})
	}

	// Сканируем директорию с БД файлами
	files, err := filepath.Glob("*.db")
	if err == nil {
		for _, file := range files {
			// Пропускаем если это текущая БД или service.db
			if file == s.currentDBPath || file == "service.db" {
				continue
			}

			databases = append(databases, map[string]interface{}{
				"path": file,
				"name": filepath.Base(file),
				"type": "available",
			})
		}
	}

	s.writeJSONResponse(w, databases, http.StatusOK)
}

// handleNormalizationTables возвращает список таблиц в указанной БД
func (s *Server) handleNormalizationTables(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	dbPath := r.URL.Query().Get("database")
	if dbPath == "" {
		// Если путь не указан, используем текущую БД
		dbPath = s.currentDBPath
	}

	// Открываем БД
	conn, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Printf("Error opening database %s: %v", dbPath, err)
		s.writeJSONError(w, fmt.Sprintf("Failed to open database: %v", err), http.StatusInternalServerError)
		return
	}
	defer conn.Close()

	// Получаем список таблиц
	rows, err := conn.Query("SELECT name FROM sqlite_master WHERE type='table' ORDER BY name")
	if err != nil {
		log.Printf("Error querying tables: %v", err)
		s.writeJSONError(w, fmt.Sprintf("Failed to query tables: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	tables := []map[string]interface{}{}
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			continue
		}

		// Пропускаем системные таблицы SQLite
		if strings.HasPrefix(tableName, "sqlite_") {
			continue
		}

		// Получаем количество записей
		var count int
		conn.QueryRow(fmt.Sprintf("SELECT COUNT(*) FROM %s", tableName)).Scan(&count)

		tables = append(tables, map[string]interface{}{
			"name":  tableName,
			"count": count,
		})
	}

	s.writeJSONResponse(w, tables, http.StatusOK)
}

// handleNormalizationColumns возвращает список колонок в указанной таблице
func (s *Server) handleNormalizationColumns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	dbPath := r.URL.Query().Get("database")
	tableName := r.URL.Query().Get("table")

	if tableName == "" {
		s.writeJSONError(w, "Table name is required", http.StatusBadRequest)
		return
	}

	if dbPath == "" {
		dbPath = s.currentDBPath
	}

	// Открываем БД
	conn, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		log.Printf("Error opening database %s: %v", dbPath, err)
		s.writeJSONError(w, fmt.Sprintf("Failed to open database: %v", err), http.StatusInternalServerError)
		return
	}
	defer conn.Close()

	// Получаем информацию о колонках
	rows, err := conn.Query(fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	if err != nil {
		log.Printf("Error querying columns: %v", err)
		s.writeJSONError(w, fmt.Sprintf("Failed to query columns: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	columns := []map[string]interface{}{}
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull, pk int
		var dfltValue sql.NullString

		if err := rows.Scan(&cid, &name, &colType, &notNull, &dfltValue, &pk); err != nil {
			continue
		}

		columns = append(columns, map[string]interface{}{
			"name":        name,
			"type":        colType,
			"primary_key": pk == 1,
			"not_null":    notNull == 1,
		})
	}

	s.writeJSONResponse(w, columns, http.StatusOK)
}

// handleKpvedHierarchy возвращает иерархию КПВЭД классификатора
func (s *Server) handleKpvedHierarchy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Получаем параметры
	parentCode := r.URL.Query().Get("parent")
	level := r.URL.Query().Get("level")
	// Используем сервисную БД для классификатора КПВЭД
	db := s.serviceDB.GetDB()

	// Строим запрос
	query := "SELECT code, name, parent_code, level FROM kpved_classifier WHERE 1=1"
	args := []interface{}{}

	if parentCode != "" {
		query += " AND parent_code = ?"
		args = append(args, parentCode)
	} else if level != "" {
		// Если указан уровень, но нет родителя - показываем этот уровень
		query += " AND level = ?"
		levelInt, _ := strconv.Atoi(level)
		args = append(args, levelInt)
	} else {
		// По умолчанию показываем верхний уровень (секции A-Z, level = 1)
		// Секции имеют parent_code = NULL или parent_code = ''
		query += " AND level = 1 AND (parent_code IS NULL OR parent_code = '')"
	}

	query += " ORDER BY code"

	rows, err := db.Query(query, args...)
	if err != nil {
		log.Printf("Error querying kpved hierarchy: %v", err)
		s.writeJSONError(w, "Failed to fetch KPVED hierarchy", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	nodes := []map[string]interface{}{}
	for rows.Next() {
		var code, name string
		var parentCode sql.NullString
		var level int

		if err := rows.Scan(&code, &name, &parentCode, &level); err != nil {
			log.Printf("Error scanning kpved row: %v", err)
			continue
		}

		// Проверяем, есть ли дочерние узлы
		var hasChildren bool
		childQuery := "SELECT COUNT(*) FROM kpved_classifier WHERE parent_code = ?"
		var childCount int
		if err := db.QueryRow(childQuery, code).Scan(&childCount); err == nil {
			hasChildren = childCount > 0
		}

		node := map[string]interface{}{
			"code":         code,
			"name":         name,
			"level":        level,
			"has_children": hasChildren,
		}
		if parentCode.Valid {
			node["parent_code"] = parentCode.String
		}

		nodes = append(nodes, node)
	}

	// Формируем ответ в формате, ожидаемом фронтендом
	response := map[string]interface{}{
		"nodes": nodes,
		"total": len(nodes),
	}

	s.writeJSONResponse(w, response, http.StatusOK)
}

// handleKpvedSearch выполняет поиск по КПВЭД классификатору
func (s *Server) handleKpvedSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	searchQuery := r.URL.Query().Get("q")
	if searchQuery == "" {
		s.writeJSONError(w, "Search query is required", http.StatusBadRequest)
		return
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	// Используем сервисную БД для классификатора КПВЭД
	db := s.serviceDB.GetDB()

	query := `
		SELECT code, name, parent_code, level
		FROM kpved_classifier
		WHERE name LIKE ? OR code LIKE ?
		ORDER BY level, code
		LIMIT ?
	`

	searchParam := "%" + searchQuery + "%"
	rows, err := db.Query(query, searchParam, searchParam, limit)
	if err != nil {
		log.Printf("Error searching kpved: %v", err)
		s.writeJSONError(w, "Failed to search KPVED", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	items := []map[string]interface{}{}
	for rows.Next() {
		var code, name string
		var parentCode sql.NullString
		var level int

		if err := rows.Scan(&code, &name, &parentCode, &level); err != nil {
			log.Printf("Error scanning kpved row: %v", err)
			continue
		}

		item := map[string]interface{}{
			"code":  code,
			"name":  name,
			"level": level,
		}
		if parentCode.Valid {
			item["parent_code"] = parentCode.String
		}

		items = append(items, item)
	}

	s.writeJSONResponse(w, items, http.StatusOK)
}

// handleKpvedStats возвращает статистику по использованию КПВЭД кодов
func (s *Server) handleKpvedStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Используем сервисную БД для классификатора КПВЭД
	db := s.serviceDB.GetDB()

	// Получаем общее количество записей в классификаторе
	var totalCodes int
	err := db.QueryRow("SELECT COUNT(*) FROM kpved_classifier").Scan(&totalCodes)
	if err != nil {
		log.Printf("Error counting kpved codes: %v", err)
		totalCodes = 0
	}

	// Получаем максимальный уровень в классификаторе
	var maxLevel int
	err = db.QueryRow("SELECT MAX(level) FROM kpved_classifier").Scan(&maxLevel)
	if err != nil {
		log.Printf("Error getting max level: %v", err)
		maxLevel = 0
	}

	// Получаем распределение по уровням
	levelsQuery := `
		SELECT level, COUNT(*) as count
		FROM kpved_classifier
		GROUP BY level
		ORDER BY level
	`
	levelsRows, err := db.Query(levelsQuery)
	if err != nil {
		log.Printf("Error querying kpved levels: %v", err)
	}
	defer levelsRows.Close()

	levels := []map[string]interface{}{}
	if levelsRows != nil {
		for levelsRows.Next() {
			var level, count int
			if err := levelsRows.Scan(&level, &count); err == nil {
				levels = append(levels, map[string]interface{}{
					"level": level,
					"count": count,
				})
			}
		}
	}

	// Формируем упрощенную статистику для фронтенда
	stats := map[string]interface{}{
		"total":               totalCodes,
		"levels":              maxLevel + 1, // +1 потому что уровни начинаются с 0
		"levels_distribution": levels,
	}

	s.writeJSONResponse(w, stats, http.StatusOK)
}

// handleKpvedLoad загружает классификатор КПВЭД из файла в базу данных
func (s *Server) handleKpvedLoad(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Читаем тело запроса
	var req struct {
		FilePath string `json:"file_path"`
		Database string `json:"database,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.FilePath == "" {
		s.writeJSONError(w, "file_path is required", http.StatusBadRequest)
		return
	}

	// Проверяем существование файла
	if _, err := os.Stat(req.FilePath); os.IsNotExist(err) {
		s.writeJSONError(w, fmt.Sprintf("File not found: %s", req.FilePath), http.StatusNotFound)
		return
	}

	// Используем сервисную БД для классификатора КПВЭД
	log.Printf("Loading KPVED classifier from file: %s to service database", req.FilePath)
	if err := database.LoadKpvedFromFile(s.serviceDB.GetDB(), req.FilePath); err != nil {
		log.Printf("Error loading KPVED: %v", err)
		s.writeJSONError(w, fmt.Sprintf("Failed to load KPVED: %v", err), http.StatusInternalServerError)
		return
	}

	// Получаем статистику после загрузки
	var totalCodes int
	err := s.serviceDB.QueryRow("SELECT COUNT(*) FROM kpved_classifier").Scan(&totalCodes)
	if err != nil {
		log.Printf("Error counting kpved codes: %v", err)
		totalCodes = 0
	}

	response := map[string]interface{}{
		"success":     true,
		"message":     "KPVED classifier loaded successfully",
		"file_path":   req.FilePath,
		"total_codes": totalCodes,
	}

	s.writeJSONResponse(w, response, http.StatusOK)
}

// handleKpvedClassifyTest тестирует КПВЭД классификацию для одного товара
func (s *Server) handleKpvedClassifyTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Читаем тело запроса
	var req struct {
		NormalizedName string `json:"normalized_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.NormalizedName == "" {
		http.Error(w, "normalized_name is required", http.StatusBadRequest)
		return
	}

	// Проверяем, что нормализатор существует и AI включен
	if s.normalizer == nil {
		http.Error(w, "Normalizer not initialized", http.StatusInternalServerError)
		return
	}

	// Получаем КПВЭД классификатор из нормализатора
	// Для этого нужно обратиться к приватному полю, что не идеально
	// Но для тестирования это приемлемо
	apiKey := os.Getenv("ARLIAI_API_KEY")
	if apiKey == "" {
		http.Error(w, "AI API key not configured", http.StatusServiceUnavailable)
		return
	}

	// Получаем модель из WorkerConfigManager
	model := s.getModelFromConfig()

	// Создаем временный классификатор для теста
	classifier := normalization.NewKpvedClassifier(s.normalizedDB, apiKey, "КПВЭД.txt", model)
	result, err := classifier.ClassifyWithKpved(req.NormalizedName)
	if err != nil {
		log.Printf("Error classifying: %v", err)
		http.Error(w, fmt.Sprintf("Classification failed: %v", err), http.StatusInternalServerError)
		return
	}

	s.writeJSONResponse(w, result, http.StatusOK)
}

// handleKpvedReclassify переклассифицирует существующие группы
func (s *Server) handleKpvedReclassify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Читаем тело запроса
	var req struct {
		Limit int `json:"limit"` // Количество групп для переклассификации (0 = все)
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.Limit = 10 // По умолчанию 10 групп
	}

	apiKey := os.Getenv("ARLIAI_API_KEY")
	if apiKey == "" {
		http.Error(w, "AI API key not configured", http.StatusServiceUnavailable)
		return
	}

	// Получаем группы без КПВЭД классификации
	query := `
		SELECT DISTINCT normalized_name, category
		FROM normalized_data
		WHERE (kpved_code IS NULL OR kpved_code = '' OR TRIM(kpved_code) = '')
		LIMIT ?
	`

	limitValue := req.Limit
	if limitValue == 0 {
		limitValue = 1000000 // Большое число для "все"
	}

	rows, err := s.db.Query(query, limitValue)
	if err != nil {
		log.Printf("Error querying groups: %v", err)
		http.Error(w, "Failed to query groups", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	// Получаем модель из WorkerConfigManager
	model := s.getModelFromConfig()

	// Создаем классификатор
	classifier := normalization.NewKpvedClassifier(s.normalizedDB, apiKey, "КПВЭД.txt", model)

	classified := 0
	failed := 0
	results := []map[string]interface{}{}

	for rows.Next() {
		var normalizedName, category string
		if err := rows.Scan(&normalizedName, &category); err != nil {
			continue
		}

		// Классифицируем
		result, err := classifier.ClassifyWithKpved(normalizedName)
		if err != nil {
			log.Printf("Failed to classify '%s': %v", normalizedName, err)
			failed++
			continue
		}

		// Обновляем все записи в этой группе
		updateQuery := `
			UPDATE normalized_data
			SET kpved_code = ?, kpved_name = ?, kpved_confidence = ?
			WHERE normalized_name = ? AND category = ?
		`
		_, err = s.db.Exec(updateQuery, result.KpvedCode, result.KpvedName, result.KpvedConfidence, normalizedName, category)
		if err != nil {
			log.Printf("Failed to update group '%s': %v", normalizedName, err)
			failed++
			continue
		}

		classified++
		results = append(results, map[string]interface{}{
			"normalized_name":  normalizedName,
			"category":         category,
			"kpved_code":       result.KpvedCode,
			"kpved_name":       result.KpvedName,
			"kpved_confidence": result.KpvedConfidence,
		})

		// Логируем прогресс
		if classified%10 == 0 {
			log.Printf("Reclassified %d groups...", classified)
		}
	}

	response := map[string]interface{}{
		"classified": classified,
		"failed":     failed,
		"results":    results,
	}

	s.writeJSONResponse(w, response, http.StatusOK)
}

// handleKpvedClassifyHierarchical выполняет иерархическую классификацию для тестирования
func (s *Server) handleKpvedClassifyHierarchical(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Читаем тело запроса
	var req struct {
		NormalizedName string `json:"normalized_name"`
		Category       string `json:"category"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.NormalizedName == "" {
		http.Error(w, "normalized_name is required", http.StatusBadRequest)
		return
	}

	// Используем "общее" как категорию по умолчанию
	if req.Category == "" {
		req.Category = "общее"
	}

	// Получаем API ключ и модель из WorkerConfigManager
	apiKey, model, err := s.workerConfigManager.GetModelAndAPIKey()
	if err != nil {
		log.Printf("[KPVED Test] Error getting API key and model: %v", err)
		http.Error(w, fmt.Sprintf("AI API key not configured: %v", err), http.StatusServiceUnavailable)
		return
	}
	log.Printf("[KPVED Test] Using API key and model: %s", model)

	// Создаем AI клиент
	aiClient := nomenclature.NewAIClient(apiKey, model)

	// Создаем иерархический классификатор (используем serviceDB где находится kpved_classifier)
	hierarchicalClassifier, err := normalization.NewHierarchicalClassifier(s.serviceDB, aiClient)
	if err != nil {
		log.Printf("Error creating hierarchical classifier: %v", err)
		http.Error(w, fmt.Sprintf("Failed to create classifier: %v", err), http.StatusInternalServerError)
		return
	}

	// Классифицируем
	startTime := time.Now()
	result, err := hierarchicalClassifier.Classify(req.NormalizedName, req.Category)
	if err != nil {
		log.Printf("Error classifying: %v", err)
		http.Error(w, fmt.Sprintf("Classification failed: %v", err), http.StatusInternalServerError)
		return
	}

	// Добавляем общее время выполнения
	result.TotalDuration = time.Since(startTime).Milliseconds()

	log.Printf("Hierarchical classification completed: %s -> %s (%s) in %dms with %d steps",
		req.NormalizedName, result.FinalCode, result.FinalName, result.TotalDuration, len(result.Steps))

	s.writeJSONResponse(w, result, http.StatusOK)
}

// classificationTask представляет задачу для классификации группы
type classificationTask struct {
	normalizedName string
	category       string
	mergedCount    int // Количество дублей в группе
	index          int
}

// classificationResult представляет результат классификации
type classificationResult struct {
	task         classificationTask
	result       *normalization.HierarchicalResult
	err          error
	rowsAffected int64
}

// handleKpvedReclassifyHierarchical переклассифицирует существующие группы с иерархическим подходом
func (s *Server) handleKpvedReclassifyHierarchical(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Читаем тело запроса
	var req struct {
		Limit int `json:"limit"` // Количество групп для переклассификации (0 = все)
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.Limit = 10 // По умолчанию 10 групп
	}

	// Получаем API ключ и модель из WorkerConfigManager
	apiKey, model, err := s.workerConfigManager.GetModelAndAPIKey()
	if err != nil {
		log.Printf("[KPVED] Error getting API key and model: %v", err)
		http.Error(w, fmt.Sprintf("AI API key not configured: %v", err), http.StatusServiceUnavailable)
		return
	}
	log.Printf("[KPVED] Using API key and model: %s", model)

	// Валидация: проверяем наличие данных в normalized_data
	// ВАЖНО: normalized_data находится в основной БД (s.db), а не в normalizedDB
	var totalGroups int
	err = s.db.QueryRow("SELECT COUNT(DISTINCT normalized_name || '|' || category) FROM normalized_data").Scan(&totalGroups)
	if err != nil {
		log.Printf("[KPVED] Error counting total groups: %v", err)
	} else {
		log.Printf("[KPVED] Total groups in normalized_data: %d", totalGroups)
	}

	// Валидация: проверяем наличие таблицы kpved_classifier в сервисной БД
	var kpvedTableExists bool
	err = s.serviceDB.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM sqlite_master 
			WHERE type='table' AND name='kpved_classifier'
		)
	`).Scan(&kpvedTableExists)
	if err != nil {
		log.Printf("[KPVED] Error checking kpved_classifier table: %v", err)
	} else if !kpvedTableExists {
		log.Printf("[KPVED] ERROR: Table kpved_classifier does not exist in service DB!")
		http.Error(w, "KPVED classifier table not found", http.StatusInternalServerError)
		return
	}

	// Валидация: проверяем количество записей в kpved_classifier
	var kpvedNodesCount int
	err = s.serviceDB.QueryRow("SELECT COUNT(*) FROM kpved_classifier").Scan(&kpvedNodesCount)
	if err != nil {
		log.Printf("[KPVED] Error counting kpved_classifier nodes: %v", err)
	} else {
		log.Printf("[KPVED] KPVED classifier nodes in database: %d", kpvedNodesCount)
		if kpvedNodesCount == 0 {
			log.Printf("[KPVED] ERROR: kpved_classifier table is empty!")
			errorMsg := "Таблица kpved_classifier пуста. Загрузите классификатор КПВЭД через эндпоинт /api/kpved/load-from-file или используйте файл КПВЭД.txt"
			http.Error(w, errorMsg, http.StatusInternalServerError)
			return
		}
	}

	// Получаем группы без КПВЭД классификации, отсортированные по количеству дублей (сначала группы с наибольшим количеством)
	query := `
		SELECT normalized_name, category, MAX(merged_count) as merged_count
		FROM normalized_data
		WHERE (kpved_code IS NULL OR kpved_code = '' OR TRIM(kpved_code) = '')
		GROUP BY normalized_name, category
		ORDER BY merged_count DESC
		LIMIT ?
	`

	limitValue := req.Limit
	if limitValue == 0 {
		limitValue = 1000000 // Большое число для "все"
	}

	log.Printf("[KPVED] Querying groups without KPVED classification (limit: %d, sorted by merged_count DESC)...", limitValue)
	// ВАЖНО: normalized_data находится в основной БД (s.db), а не в normalizedDB
	rows, err := s.db.Query(query, limitValue)
	if err != nil {
		log.Printf("[KPVED] Error querying groups: %v", err)
		http.Error(w, "Failed to query groups", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	// Создаем AI клиент и иерархический классификатор
	log.Printf("[KPVED] Creating hierarchical classifier with API key (length: %d) and model: %s", len(apiKey), model)
	if apiKey == "" {
		log.Printf("[KPVED] ERROR: API key is empty!")
		http.Error(w, "API ключ не настроен. Настройте API ключ в конфигурации воркеров или переменной окружения ARLIAI_API_KEY", http.StatusServiceUnavailable)
		return
	}

	log.Printf("[KPVED] Initializing AI client and hierarchical classifier...")
	
	// ВАЖНО: Проверяем, не работает ли нормализация одновременно
	// Если работает, это может привести к превышению лимита параллельных запросов
	s.normalizerMutex.RLock()
	isNormalizerRunning := s.normalizerRunning
	s.normalizerMutex.RUnlock()
	
	if isNormalizerRunning {
		log.Printf("[KPVED] WARNING: Normalizer is running! This may cause exceeding Arliai API limit (2 parallel calls for ADVANCED plan)")
		log.Printf("[KPVED] Consider stopping normalization before starting KPVED classification")
	}
	
	// Создаем один AI клиент и один hierarchical classifier для всех воркеров
	// ВАЖНО: Все воркеры будут использовать один и тот же aiClient экземпляр
	// Это важно, так как rate limiter работает на уровне экземпляра AIClient
	aiClient := nomenclature.NewAIClient(apiKey, model)
	log.Printf("[KPVED] Created AI client instance (rate limiter: 1 req/sec, burst: 5)")
	
	hierarchicalClassifier, err := normalization.NewHierarchicalClassifier(s.serviceDB, aiClient)
	if err != nil {
		log.Printf("[KPVED] ERROR creating hierarchical classifier: %v", err)
		errorMsg := fmt.Sprintf("Не удалось создать классификатор: %v. Проверьте, что таблица kpved_classifier загружена и содержит данные.", err)
		http.Error(w, errorMsg, http.StatusInternalServerError)
		return
	}
	log.Printf("[KPVED] Hierarchical classifier created successfully (will be shared by all workers)")

	// Получаем статистику дерева КПВЭД
	cacheStats := hierarchicalClassifier.GetCacheStats()
	log.Printf("[KPVED] Hierarchical classifier created successfully. Cache stats: %+v", cacheStats)

	// Сначала подсчитываем общее количество групп без КПВЭД (без учета лимита)
	var totalGroupsWithoutKpved int
	countQuery := `
		SELECT COUNT(DISTINCT normalized_name || '|' || category)
		FROM normalized_data
		WHERE (kpved_code IS NULL OR kpved_code = '' OR TRIM(kpved_code) = '')
	`
	// ВАЖНО: normalized_data находится в основной БД (s.db), а не в normalizedDB
	err = s.db.QueryRow(countQuery).Scan(&totalGroupsWithoutKpved)
	if err != nil {
		log.Printf("[KPVED] Error counting groups without KPVED: %v", err)
		totalGroupsWithoutKpved = 0
	} else {
		log.Printf("[KPVED] Total groups without KPVED: %d (limit: %d)", totalGroupsWithoutKpved, limitValue)
	}

	// Проверяем на пустой результат
	if totalGroupsWithoutKpved == 0 {
		log.Printf("[KPVED] WARNING: No groups found without KPVED classification!")
		// Проверяем, есть ли вообще группы с KPVED
		var groupsWithKpved int
		var totalGroups int
		// ВАЖНО: normalized_data находится в основной БД (s.db), а не в normalizedDB
		err = s.db.QueryRow(`
			SELECT COUNT(DISTINCT normalized_name || '|' || category)
			FROM normalized_data
			WHERE kpved_code IS NOT NULL AND kpved_code != '' AND TRIM(kpved_code) != ''
		`).Scan(&groupsWithKpved)
		if err == nil {
			log.Printf("[KPVED] Groups with KPVED: %d, Groups without KPVED: 0", groupsWithKpved)
		}

		// Получаем общее количество групп для информативности
		err = s.db.QueryRow(`
			SELECT COUNT(DISTINCT normalized_name || '|' || category)
			FROM normalized_data
		`).Scan(&totalGroups)
		if err != nil {
			totalGroups = 0
		}

		response := map[string]interface{}{
			"classified":        0,
			"failed":            0,
			"total_duration":    0,
			"avg_duration":      0,
			"avg_steps":         0.0,
			"avg_ai_calls":      0.0,
			"total_ai_calls":    0,
			"results":           []map[string]interface{}{},
			"message":           fmt.Sprintf("Не найдено групп без классификации КПВЭД. Всего групп: %d, с классификацией: %d", totalGroups, groupsWithKpved),
			"total_groups":      totalGroups,
			"groups_with_kpved": groupsWithKpved,
		}
		s.writeJSONResponse(w, response, http.StatusOK)
		return
	}

	// Функция для retry UPDATE запросов с экспоненциальной задержкой и таймаутом
	retryUpdate := func(query string, args ...interface{}) (sql.Result, error) {
		maxRetries := 5
		baseDelay := 50 * time.Millisecond
		queryTimeout := 10 * time.Second // Таймаут для каждого запроса

		for attempt := 0; attempt < maxRetries; attempt++ {
			// Создаем context с таймаутом для запроса
			ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)

			// Выполняем запрос с таймаутом
			result, err := s.db.GetDB().ExecContext(ctx, query, args...)
			cancel() // Освобождаем ресурсы context сразу после использования

			if err == nil {
				return result, nil
			}

			// Проверяем, является ли это ошибкой блокировки БД или таймаута
			errStr := err.Error()
			isLockError := strings.Contains(errStr, "database is locked") || strings.Contains(errStr, "locked")
			isTimeoutError := strings.Contains(errStr, "timeout") || strings.Contains(errStr, "context deadline exceeded")

			if !isLockError && !isTimeoutError {
				// Если это не ошибка блокировки или таймаута, возвращаем сразу
				return nil, err
			}

			if attempt < maxRetries-1 {
				// Экспоненциальная задержка: 50ms, 100ms, 200ms, 400ms, 800ms
				delay := baseDelay * time.Duration(1<<uint(attempt))
				if isTimeoutError {
					log.Printf("[KPVED] Query timeout, retrying in %v (attempt %d/%d)...", delay, attempt+1, maxRetries)
				} else {
					log.Printf("[KPVED] Database locked, retrying in %v (attempt %d/%d)...", delay, attempt+1, maxRetries)
				}
				time.Sleep(delay)
			} else {
				log.Printf("[KPVED] Max retries reached for database update")
				return nil, err
			}
		}

		return nil, fmt.Errorf("failed after %d retries", maxRetries)
	}

	// Собираем все задачи в слайс
	var tasks []classificationTask
	index := 0
	for rows.Next() {
		var normalizedName, category string
		var mergedCount int
		if err := rows.Scan(&normalizedName, &category, &mergedCount); err != nil {
			log.Printf("[KPVED] Error scanning row: %v", err)
			continue
		}
		tasks = append(tasks, classificationTask{
			normalizedName: normalizedName,
			category:       category,
			mergedCount:    mergedCount,
			index:          index,
		})
		index++
	}

	if err := rows.Err(); err != nil {
		log.Printf("[KPVED] Error iterating rows: %v", err)
		http.Error(w, "Failed to read groups from database", http.StatusInternalServerError)
		return
	}

	if len(tasks) == 0 {
		log.Printf("[KPVED] No tasks to process")
		response := map[string]interface{}{
			"classified":     0,
			"failed":         0,
			"total_duration": 0,
			"avg_duration":   0,
			"avg_steps":      0.0,
			"avg_ai_calls":   0.0,
			"total_ai_calls": 0,
			"results":        []map[string]interface{}{},
		}
		s.writeJSONResponse(w, response, http.StatusOK)
		return
	}

	// Определяем количество воркеров
	// ВАЖНО: Arliai API ADVANCED план поддерживает только 2 параллельных вызова ВСЕГО
	// Нужно проверить, не работают ли другие процессы (нормализация), которые также используют API
	maxWorkers := 2 // Arliai API ограничение: максимум 2 параллельных вызова

	// Проверяем, не работает ли нормализация одновременно (переиспользуем переменную из строки 7509)
	s.normalizerMutex.RLock()
	isNormalizerRunning = s.normalizerRunning
	s.normalizerMutex.RUnlock()
	
	if isNormalizerRunning {
		log.Printf("[KPVED] WARNING: Normalizer is running simultaneously. Reducing workers to 1 to avoid exceeding Arliai API limit (2 parallel calls for ADVANCED plan)")
		maxWorkers = 1 // Если нормализация работает, используем только 1 воркер
	}

	// Проверяем настройки из WorkerConfigManager
	if s.workerConfigManager != nil {
		provider, err := s.workerConfigManager.GetActiveProvider()
		if err == nil && provider != nil {
			if provider.MaxWorkers > 0 && provider.MaxWorkers < maxWorkers {
				log.Printf("[KPVED] Using provider MaxWorkers=%d (requested %d)", provider.MaxWorkers, maxWorkers)
				maxWorkers = provider.MaxWorkers
			}
		}
		// Также проверяем глобальное значение
		s.workerConfigManager.mu.RLock()
		globalMaxWorkers := s.workerConfigManager.globalMaxWorkers
		s.workerConfigManager.mu.RUnlock()
		if globalMaxWorkers > 0 && globalMaxWorkers < maxWorkers {
			log.Printf("[KPVED] Using global MaxWorkers=%d (requested %d)", globalMaxWorkers, maxWorkers)
			maxWorkers = globalMaxWorkers
		}
	}

	// ВАЖНО: Arliai API ADVANCED план поддерживает только 2 параллельных вызова ВСЕГО
	// Не превышаем лимит, но используем максимум доступных воркеров
	if maxWorkers > 2 {
		log.Printf("[KPVED] WARNING: Requested %d workers, but Arliai API ADVANCED plan supports only 2 parallel calls TOTAL. Limiting to 2 workers.", maxWorkers)
		maxWorkers = 2
	}
	// Обеспечиваем минимум 1 воркер
	if maxWorkers < 1 {
		maxWorkers = 1
	}
	
	log.Printf("[KPVED] Using %d workers for classification (normalizer running: %v, Arliai API limit: 2 parallel calls)", maxWorkers, isNormalizerRunning)
	log.Printf("[KPVED] IMPORTANT: All %d workers will share the same AI client instance to respect rate limits", maxWorkers)
	
	if isNormalizerRunning && maxWorkers >= 2 {
		log.Printf("[KPVED] CRITICAL: Normalizer is running AND using %d workers! This will likely exceed Arliai API limit (2 parallel calls)", maxWorkers)
		log.Printf("[KPVED] Total parallel requests: %d (normalizer) + %d (KPVED workers) = %d (limit: 2)", 1, maxWorkers, maxWorkers+1)
	}

	log.Printf("[KPVED] Starting classification with %d workers for %d groups (sorted by merged_count DESC)", maxWorkers, len(tasks))

	// Создаем каналы для задач и результатов
	// Ограничиваем буфер канала, чтобы не загружать все задачи сразу
	// Это предотвращает одновременную отправку большого количества запросов
	taskChan := make(chan classificationTask, maxWorkers*2) // Буфер только для 2 задач
	resultChan := make(chan classificationResult, maxWorkers*2)
	var wg sync.WaitGroup

	// Очищаем map для отслеживания текущих задач перед началом новой классификации
	s.kpvedCurrentTasksMutex.Lock()
	s.kpvedCurrentTasks = make(map[int]*classificationTask)
	s.kpvedCurrentTasksMutex.Unlock()

	// Сбрасываем флаг остановки при начале новой классификации
	s.kpvedWorkersStopMutex.Lock()
	s.kpvedWorkersStopped = false
	s.kpvedWorkersStopMutex.Unlock()

	for i := 0; i < maxWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for task := range taskChan {
				// Проверяем флаг остановки перед обработкой задачи
				s.kpvedWorkersStopMutex.RLock()
				stopped := s.kpvedWorkersStopped
				s.kpvedWorkersStopMutex.RUnlock()

				if stopped {
					log.Printf("[KPVED Worker %d] Stopped by user, skipping task %d: '%s'", workerID, task.index, task.normalizedName)
					// Удаляем задачу из отслеживания
					s.kpvedCurrentTasksMutex.Lock()
					delete(s.kpvedCurrentTasks, workerID)
					s.kpvedCurrentTasksMutex.Unlock()
					// Отправляем результат с ошибкой остановки
					resultChan <- classificationResult{
						task: task,
						err:  fmt.Errorf("worker stopped by user"),
					}
					continue
				}

				// Обновляем текущую задачу для этого воркера
				s.kpvedCurrentTasksMutex.Lock()
				s.kpvedCurrentTasks[workerID] = &task
				s.kpvedCurrentTasksMutex.Unlock()

				// Логируем каждую задачу для отслеживания параллелизма (только каждую 100-ю для уменьшения логов)
				if task.index%100 == 0 {
					log.Printf("[KPVED Worker %d] Processing task %d: '%s' in category '%s' (merged_count: %d)",
						workerID, task.index, task.normalizedName, task.category, task.mergedCount)
				}

				// Классифицируем с иерархическим подходом
				// Логируем только каждую 10-ю задачу для уменьшения объема логов
				if task.index%10 == 0 {
					log.Printf("[KPVED Worker %d] Starting classification for '%s' (category: '%s')", workerID, task.normalizedName, task.category)
				}

				result, err := hierarchicalClassifier.Classify(task.normalizedName, task.category)

				if err != nil {
					// Проверяем тип ошибки
					errStr := err.Error()
					isRateLimit := strings.Contains(errStr, "rate limit") ||
						strings.Contains(errStr, "too many requests") ||
						strings.Contains(errStr, "429") ||
						strings.Contains(errStr, "quota exceeded") ||
						strings.Contains(errStr, "exceeded the maximum number of parallel requests")

					isCircuitBreakerOpen := strings.Contains(errStr, "circuit breaker is open")

					// Если circuit breaker открыт, ждем пока он закроется
					if isCircuitBreakerOpen {
						log.Printf("[KPVED Worker %d] Circuit breaker is open, waiting for recovery (task: '%s')...", workerID, task.normalizedName)
						// Ждем 5 секунд перед повторной попыткой
						time.Sleep(5 * time.Second)
						// Пытаемся еще раз
						retryResult, retryErr := hierarchicalClassifier.Classify(task.normalizedName, task.category)
						if retryErr == nil {
							// Успешно после retry - используем результат
							result = retryResult
							err = nil
						} else {
							errStr = retryErr.Error()
							isCircuitBreakerOpen = strings.Contains(errStr, "circuit breaker is open")
							if isCircuitBreakerOpen {
								log.Printf("[KPVED Worker %d] Circuit breaker still open after retry, waiting 10 more seconds...", workerID)
								time.Sleep(10 * time.Second)
								// Последняя попытка
								finalResult, finalErr := hierarchicalClassifier.Classify(task.normalizedName, task.category)
								if finalErr == nil {
									// Успешно после последней попытки
									result = finalResult
									err = nil
								} else {
									// Все попытки неудачны
									err = finalErr
								}
							} else {
								// Другая ошибка после retry
								err = retryErr
							}
						}
					}
					
					// Если после всех retry все еще есть ошибка, обрабатываем ее
					if err != nil {
						// Обновляем errStr после возможных retry
						errStr = err.Error()
						isRateLimit = strings.Contains(errStr, "rate limit") ||
							strings.Contains(errStr, "too many requests") ||
							strings.Contains(errStr, "429") ||
							strings.Contains(errStr, "quota exceeded") ||
							strings.Contains(errStr, "exceeded the maximum number of parallel requests")

						// Если это rate limit или превышение параллельных запросов, делаем паузу
						if isRateLimit {
							log.Printf("[KPVED Worker %d] Rate limit detected, pausing for 3 seconds before retry...", workerID)
							time.Sleep(3 * time.Second)
						}
						
						// Добавляем небольшую задержку между запросами для предотвращения перегрузки API
						// Это особенно важно при использовании 1 воркера
						time.Sleep(100 * time.Millisecond)

						// Удаляем задачу из отслеживания при ошибке
						s.kpvedCurrentTasksMutex.Lock()
						delete(s.kpvedCurrentTasks, workerID)
						s.kpvedCurrentTasksMutex.Unlock()

						// Детальное логирование ошибки
						log.Printf("[KPVED Worker %d] ERROR classifying '%s' (category: '%s', merged_count: %d): %v",
							workerID, task.normalizedName, task.category, task.mergedCount, err)
						// Пытаемся извлечь более детальную информацию об ошибке
						if isRateLimit {
							log.Printf("[KPVED Worker %d]   -> Rate limit error - Arliai API limit reached, paused and will retry", workerID)
						} else if strings.Contains(errStr, "ai call failed") {
							log.Printf("[KPVED Worker %d]   -> AI call failed - check API key, network connection, or rate limits", workerID)
						} else if strings.Contains(errStr, "no candidates found") {
							log.Printf("[KPVED Worker %d]   -> No candidates found in KPVED tree - check classifier data", workerID)
						} else if strings.Contains(errStr, "json unmarshal") {
							log.Printf("[KPVED Worker %d]   -> JSON parsing error - AI response format issue", workerID)
						} else if strings.Contains(errStr, "timeout") {
							log.Printf("[KPVED Worker %d]   -> Timeout error - AI service may be slow", workerID)
						}

						resultChan <- classificationResult{
							task: task,
							err:  err,
						}
						continue
					}
				} // конец первого if err != nil

				// Обновляем все записи в этой группе с retry логикой
				// ВАЖНО: normalized_data находится в основной БД (s.db), а не в normalizedDB
				updateQuery := `
					UPDATE normalized_data
					SET kpved_code = ?, kpved_name = ?, kpved_confidence = ?
					WHERE normalized_name = ? AND category = ?
				`
				updateResult, err := retryUpdate(updateQuery, result.FinalCode, result.FinalName, result.FinalConfidence, task.normalizedName, task.category)
				if err != nil {
					// Удаляем задачу из отслеживания при ошибке обновления
					s.kpvedCurrentTasksMutex.Lock()
					delete(s.kpvedCurrentTasks, workerID)
					s.kpvedCurrentTasksMutex.Unlock()

					log.Printf("[KPVED Worker %d] Failed to update group '%s' (category: '%s') after retries: %v", workerID, task.normalizedName, task.category, err)
					resultChan <- classificationResult{
						task: task,
						err:  err,
					}
					continue
				}

				rowsAffected, _ := updateResult.RowsAffected()
				if rowsAffected == 0 {
					log.Printf("[KPVED Worker %d] WARNING: Update query affected 0 rows for group '%s' (category: '%s')", workerID, task.normalizedName, task.category)
				} else {
					log.Printf("[KPVED Worker %d] Updated %d rows for group '%s' (category: '%s') -> KPVED: %s (%s, confidence: %.2f)",
						workerID, rowsAffected, task.normalizedName, task.category, result.FinalCode, result.FinalName, result.FinalConfidence)
				}

				// Удаляем задачу из отслеживания после успешного завершения
				s.kpvedCurrentTasksMutex.Lock()
				delete(s.kpvedCurrentTasks, workerID)
				s.kpvedCurrentTasksMutex.Unlock()

				resultChan <- classificationResult{
					task:         task,
					result:       result,
					rowsAffected: rowsAffected,
				}
				
				// Добавляем задержку после успешной классификации для предотвращения перегрузки API
				// Это особенно важно при использовании 1 воркера, чтобы не превысить rate limits
				time.Sleep(200 * time.Millisecond)
			} // конец for task := range taskChan
		}(i) // конец go func(workerID int)
	}

	// Отправляем задачи в канал в отдельной горутине
	// Ограничиваем скорость загрузки задач, чтобы не перегрузить канал и не вызвать circuit breaker
	go func() {
		for i, task := range tasks {
			taskChan <- task
			// Добавляем небольшую задержку каждые 10 задач, чтобы не перегружать канал
			// Это предотвращает одновременную отправку большого количества запросов
			if i > 0 && i%10 == 0 {
				time.Sleep(100 * time.Millisecond)
			}
		}
		close(taskChan)
	}()

	// Закрываем канал результатов после завершения всех воркеров
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	// Собираем результаты в главной горутине
	classified := 0
	failed := 0
	totalDuration := int64(0)
	totalSteps := 0
	totalAICalls := 0
	results := []map[string]interface{}{}
	errorSamples := []string{} // Сохраняем первые 10 ошибок для диагностики

	for res := range resultChan {
		if res.err != nil {
			failed++
			errorMsg := res.err.Error()

			// Сохраняем первые 10 ошибок для диагностики
			if len(errorSamples) < 10 {
				errorSamples = append(errorSamples, fmt.Sprintf("'%s' (category: '%s'): %s",
					res.task.normalizedName, res.task.category, errorMsg))
			}

			// Логируем первые 10 ошибок для диагностики с детальной информацией
			if failed <= 10 {
				log.Printf("[KPVED] Error sample %d: '%s' (category: '%s', merged_count: %d) -> %v",
					failed, res.task.normalizedName, res.task.category, res.task.mergedCount, res.err)
				// Анализируем тип ошибки
				errStr := errorMsg
				if strings.Contains(errStr, "ai call failed") {
					log.Printf("[KPVED]   -> AI call failed - check API key, network, or rate limits")
				} else if strings.Contains(errStr, "no candidates found") {
					log.Printf("[KPVED]   -> No candidates in KPVED tree - check classifier data")
				} else if strings.Contains(errStr, "json unmarshal") {
					log.Printf("[KPVED]   -> JSON parsing error - AI response format issue")
				} else if strings.Contains(errStr, "timeout") {
					log.Printf("[KPVED]   -> Timeout error - AI service may be slow or unavailable")
				}
			}

			// Добавляем детальную информацию об ошибке в результаты
			results = append(results, map[string]interface{}{
				"normalized_name": res.task.normalizedName,
				"category":        res.task.category,
				"error":           errorMsg,
				"success":         false,
			})
		} else {
			classified++
			totalDuration += res.result.TotalDuration
			totalSteps += len(res.result.Steps)
			totalAICalls += res.result.AICallsCount

			results = append(results, map[string]interface{}{
				"normalized_name":  res.task.normalizedName,
				"category":         res.task.category,
				"kpved_code":       res.result.FinalCode,
				"kpved_name":       res.result.FinalName,
				"kpved_confidence": res.result.FinalConfidence,
				"steps":            len(res.result.Steps),
				"duration_ms":      res.result.TotalDuration,
				"ai_calls":         res.result.AICallsCount,
			})

			// Промежуточное обновление статуса каждые 20 групп для отображения прогресса на фронтенде
			if classified%20 == 0 {
				avgDuration := totalDuration / int64(classified)
				log.Printf("[KPVED] Progress: %d/%d classified (avg: %dms, %d AI calls, %d failed)...",
					classified+failed, len(tasks), avgDuration, totalAICalls, failed)
			}
		}
	}

	avgDuration := int64(0)
	avgSteps := 0.0
	avgAICalls := 0.0
	if classified > 0 {
		avgDuration = totalDuration / int64(classified)
		avgSteps = float64(totalSteps) / float64(classified)
		avgAICalls = float64(totalAICalls) / float64(classified)
	}

	// Формируем сообщение о результате
	var message string
	if classified > 0 && failed == 0 {
		message = fmt.Sprintf("Классификация завершена успешно! Обработано %d групп.", classified)
	} else if classified > 0 && failed > 0 {
		message = fmt.Sprintf("Классификация завершена частично. Обработано: %d, ошибок: %d", classified, failed)
	} else if classified == 0 && failed > 0 {
		message = fmt.Sprintf("Все группы (%d) завершились с ошибкой. Проверьте логи и настройки API.", failed)
	} else {
		message = "Классификация завершена, но не найдено групп для обработки."
	}

	response := map[string]interface{}{
		"classified":     classified,
		"failed":         failed,
		"total_duration": totalDuration,
		"avg_duration":   avgDuration,
		"avg_steps":      avgSteps,
		"avg_ai_calls":   avgAICalls,
		"total_ai_calls": totalAICalls,
		"results":        results,
		"message":        message,
		"total_groups":   len(tasks),
	}

	log.Printf("[KPVED] Hierarchical reclassification completed: %d classified, %d failed out of %d total, avg %dms/item",
		classified, failed, len(tasks), avgDuration)

	// Очищаем отслеживание задач после завершения всей классификации
	s.kpvedCurrentTasksMutex.Lock()
	s.kpvedCurrentTasks = make(map[int]*classificationTask)
	s.kpvedCurrentTasksMutex.Unlock()

	// Сбрасываем флаг остановки после завершения классификации
	s.kpvedWorkersStopMutex.Lock()
	s.kpvedWorkersStopped = false
	s.kpvedWorkersStopMutex.Unlock()

	if failed > 0 && classified == 0 {
		log.Printf("[KPVED] ERROR: All %d groups failed classification! Check logs above for details.", failed)
		if len(errorSamples) > 0 {
			log.Printf("[KPVED] First %d error samples:", len(errorSamples))
			for i, sample := range errorSamples {
				log.Printf("[KPVED]   %d. %s", i+1, sample)
			}
			// Обновляем сообщение с примерами ошибок
			sampleCount := min(len(errorSamples), 3)
			if sampleCount > 0 {
				message = fmt.Sprintf("Все группы (%d) завершились с ошибкой. Примеры ошибок:\n%s\n\nПроверьте логи сервера для деталей. Возможные причины: неверный API ключ, отсутствие данных КПВЭД классификатора, проблемы с сетью.",
					failed, strings.Join(errorSamples[:sampleCount], "\n"))
				// Добавляем примеры ошибок в ответ
				response["error_samples"] = errorSamples[:sampleCount]
			}
			// Обновляем message в response
			response["message"] = message
		}
	}

	s.writeJSONResponse(w, response, http.StatusOK)
}

// handleKpvedCurrentTasks возвращает текущие обрабатываемые задачи
func (s *Server) handleKpvedCurrentTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.kpvedCurrentTasksMutex.RLock()
	defer s.kpvedCurrentTasksMutex.RUnlock()

	// Преобразуем map в массив для JSON
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

	response := map[string]interface{}{
		"current_tasks": currentTasks,
		"count":         len(currentTasks),
	}

	s.writeJSONResponse(w, response, http.StatusOK)
}

// ============================================================================
// Quality Endpoints Handlers
// ============================================================================

// handleQualityUploadRoutes обрабатывает маршруты качества для выгрузок
func (s *Server) handleQualityUploadRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/upload/")
	parts := strings.Split(path, "/")

	if len(parts) < 2 {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	uploadUUID := parts[0]
	action := parts[1]

	switch action {
	case "quality-report":
		if r.Method == http.MethodGet {
			s.handleQualityReport(w, r, uploadUUID)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	case "quality-analysis":
		if r.Method == http.MethodPost {
			s.handleQualityAnalysis(w, r, uploadUUID)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	default:
		// Пропускаем другие маршруты
		return
	}
}

// handleQualityDatabaseRoutes обрабатывает маршруты качества для баз данных
func (s *Server) handleQualityDatabaseRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/databases/")
	parts := strings.Split(path, "/")

	if len(parts) < 2 {
		// Пропускаем другие маршруты баз данных - передаем в handleDatabaseV1Routes
		s.handleDatabaseV1Routes(w, r)
		return
	}

	databaseIDStr := parts[0]
	action := parts[1]

	// Проверяем, что это маршрут качества
	if action != "quality-dashboard" && action != "quality-issues" && action != "quality-trends" {
		// Пропускаем другие маршруты - передаем в handleDatabaseV1Routes
		s.handleDatabaseV1Routes(w, r)
		return
	}

	databaseID, err := strconv.Atoi(databaseIDStr)
	if err != nil {
		http.Error(w, "Invalid database ID", http.StatusBadRequest)
		return
	}

	switch action {
	case "quality-dashboard":
		if r.Method == http.MethodGet {
			s.handleQualityDashboard(w, r, databaseID)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	case "quality-issues":
		if r.Method == http.MethodGet {
			s.handleQualityIssues(w, r, databaseID)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	case "quality-trends":
		if r.Method == http.MethodGet {
			s.handleQualityTrends(w, r, databaseID)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	default:
		// Пропускаем другие маршруты - передаем в handleDatabaseV1Routes
		s.handleDatabaseV1Routes(w, r)
	}
}

// handleQualityReport возвращает отчет о качестве выгрузки
func (s *Server) handleQualityReport(w http.ResponseWriter, r *http.Request, uploadUUID string) {
	upload, err := s.db.GetUploadByUUID(uploadUUID)
	if err != nil {
		s.writeJSONError(w, "Upload not found", http.StatusNotFound)
		return
	}

	// Парсим параметры запроса
	summaryOnly := r.URL.Query().Get("summary_only") == "true"
	maxIssuesStr := r.URL.Query().Get("max_issues")
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")

	// Получаем метрики качества
	metrics, err := s.db.GetQualityMetrics(upload.ID)
	if err != nil {
		s.writeJSONError(w, "Failed to get quality metrics", http.StatusInternalServerError)
		return
	}

	// Определяем параметры пагинации
	limit := 0
	offset := 0
	if maxIssuesStr != "" {
		if max, err := strconv.Atoi(maxIssuesStr); err == nil && max > 0 {
			limit = max
		}
	}
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}
	if offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	// Если summary_only=true, не загружаем issues
	var issues []database.DataQualityIssue
	var totalIssuesCount int
	if !summaryOnly {
		// Получаем проблемы качества с пагинацией
		issues, totalIssuesCount, err = s.db.GetQualityIssues(upload.ID, map[string]interface{}{}, limit, offset)
		if err != nil {
			s.writeJSONError(w, "Failed to get quality issues", http.StatusInternalServerError)
			return
		}
	} else {
		// Для сводки получаем только количество без деталей
		_, totalIssuesCount, err = s.db.GetQualityIssues(upload.ID, map[string]interface{}{}, 0, 0)
		if err != nil {
			s.writeJSONError(w, "Failed to count quality issues", http.StatusInternalServerError)
			return
		}
		issues = []database.DataQualityIssue{} // Пустой список
	}

	// Формируем сводку
	summary := QualitySummary{
		TotalIssues:       totalIssuesCount,
		MetricsByCategory: make(map[string]float64),
	}

	// Подсчитываем проблемы по уровням серьезности
	// Если summary_only, используем только загруженные issues для подсчета
	// В противном случае нужно получить статистику отдельным запросом
	if !summaryOnly {
		for _, issue := range issues {
			switch issue.IssueSeverity {
			case "CRITICAL":
				summary.CriticalIssues++
			case "HIGH":
				summary.HighIssues++
			case "MEDIUM":
				summary.MediumIssues++
			case "LOW":
				summary.LowIssues++
			}
		}
	} else {
		// Для summary_only получаем статистику по уровням отдельным запросом
		severityStats, err := s.getIssuesSeverityStats(upload.ID)
		if err == nil {
			summary.CriticalIssues = severityStats["CRITICAL"]
			summary.HighIssues = severityStats["HIGH"]
			summary.MediumIssues = severityStats["MEDIUM"]
			summary.LowIssues = severityStats["LOW"]
		}
	}

	// Группируем метрики по категориям
	for _, metric := range metrics {
		if _, exists := summary.MetricsByCategory[metric.MetricCategory]; !exists {
			summary.MetricsByCategory[metric.MetricCategory] = 0.0
		}
		summary.MetricsByCategory[metric.MetricCategory] += metric.MetricValue
	}

	// Рассчитываем средние значения
	for category := range summary.MetricsByCategory {
		count := 0
		for _, metric := range metrics {
			if metric.MetricCategory == category {
				count++
			}
		}
		if count > 0 {
			summary.MetricsByCategory[category] = summary.MetricsByCategory[category] / float64(count)
		}
	}

	databaseID := 0
	if upload.DatabaseID != nil {
		databaseID = *upload.DatabaseID
	}

	report := QualityReport{
		UploadUUID:   uploadUUID,
		DatabaseID:   databaseID,
		AnalyzedAt:   upload.CompletedAt,
		OverallScore: 0.0,
		Metrics:      metrics,
		Issues:       issues,
		Summary:      summary,
	}

	// Рассчитываем общий балл
	if len(metrics) > 0 {
		var totalScore float64
		for _, metric := range metrics {
			totalScore += metric.MetricValue
		}
		report.OverallScore = totalScore / float64(len(metrics))
	}

	// Добавляем метаданные пагинации, если используется пагинация
	response := map[string]interface{}{
		"upload_uuid":   report.UploadUUID,
		"database_id":   report.DatabaseID,
		"analyzed_at":   report.AnalyzedAt,
		"overall_score": report.OverallScore,
		"metrics":       report.Metrics,
		"issues":        report.Issues,
		"summary":       report.Summary,
	}

	if limit > 0 {
		response["pagination"] = map[string]interface{}{
			"limit":       limit,
			"offset":      offset,
			"total_count": totalIssuesCount,
			"returned":    len(issues),
			"has_more":    offset+len(issues) < totalIssuesCount,
		}
	}

	s.writeJSONResponse(w, response, http.StatusOK)
}

// getIssuesSeverityStats получает статистику по уровням серьезности проблем
func (s *Server) getIssuesSeverityStats(uploadID int) (map[string]int, error) {
	query := `
		SELECT issue_severity, COUNT(*) as count
		FROM data_quality_issues
		WHERE upload_id = ?
		GROUP BY issue_severity
	`

	rows, err := s.db.Query(query, uploadID)
	if err != nil {
		return nil, fmt.Errorf("failed to query severity stats: %w", err)
	}
	defer rows.Close()

	stats := map[string]int{
		"CRITICAL": 0,
		"HIGH":     0,
		"MEDIUM":   0,
		"LOW":      0,
	}

	for rows.Next() {
		var severity string
		var count int
		if err := rows.Scan(&severity, &count); err != nil {
			continue
		}
		stats[severity] = count
	}

	return stats, nil
}

// handleQualityAnalysis запускает анализ качества для выгрузки
func (s *Server) handleQualityAnalysis(w http.ResponseWriter, r *http.Request, uploadUUID string) {
	upload, err := s.db.GetUploadByUUID(uploadUUID)
	if err != nil {
		s.writeJSONError(w, "Upload not found", http.StatusNotFound)
		return
	}

	databaseID := 0
	if upload.DatabaseID != nil {
		databaseID = *upload.DatabaseID
	}

	if databaseID == 0 {
		s.writeJSONError(w, "Database ID not set for upload", http.StatusBadRequest)
		return
	}

	// Запускаем анализ в фоне
	go func() {
		log.Printf("Starting quality analysis for upload %s (ID: %d, Database: %d)", uploadUUID, upload.ID, databaseID)
		if err := s.qualityAnalyzer.AnalyzeUpload(upload.ID, databaseID); err != nil {
			log.Printf("Quality analysis failed for upload %s: %v", uploadUUID, err)
		} else {
			log.Printf("Quality analysis completed for upload %s", uploadUUID)
		}
	}()

	response := map[string]interface{}{
		"status":  "analysis_started",
		"message": "Quality analysis started in background",
	}

	s.writeJSONResponse(w, response, http.StatusOK)
}

// handleQualityDashboard возвращает дашборд качества для базы данных
func (s *Server) handleQualityDashboard(w http.ResponseWriter, r *http.Request, databaseID int) {
	// Получаем тренды качества
	days := 30
	if daysStr := r.URL.Query().Get("days"); daysStr != "" {
		if d, err := strconv.Atoi(daysStr); err == nil && d > 0 {
			days = d
		}
	}

	trends, err := s.db.GetQualityTrends(databaseID, days)
	if err != nil {
		s.writeJSONError(w, "Failed to get quality trends", http.StatusInternalServerError)
		return
	}

	// Текущие метрики
	currentMetrics, err := s.db.GetCurrentQualityMetrics(databaseID)
	if err != nil {
		s.writeJSONError(w, "Failed to get current metrics", http.StatusInternalServerError)
		return
	}

	// Топ проблем
	limit := 10
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	topIssues, err := s.db.GetTopQualityIssues(databaseID, limit)
	if err != nil {
		s.writeJSONError(w, "Failed to get top issues", http.StatusInternalServerError)
		return
	}

	// Группируем метрики по сущностям
	metricsByEntity := make(map[string]EntityMetrics)

	for _, metric := range currentMetrics {
		// Определяем тип сущности из имени метрики
		entityType := "unknown"
		if strings.Contains(metric.MetricName, "nomenclature") {
			entityType = "nomenclature"
		} else if strings.Contains(metric.MetricName, "counterparty") {
			entityType = "counterparty"
		}

		if _, exists := metricsByEntity[entityType]; !exists {
			metricsByEntity[entityType] = EntityMetrics{}
		}

		entityMetrics := metricsByEntity[entityType]
		switch metric.MetricCategory {
		case "completeness":
			entityMetrics.Completeness = metric.MetricValue
		case "consistency":
			entityMetrics.Consistency = metric.MetricValue
		case "uniqueness":
			entityMetrics.Uniqueness = metric.MetricValue
		case "validity":
			entityMetrics.Validity = metric.MetricValue
		}

		// Рассчитываем общий балл
		count := 0
		total := 0.0
		if entityMetrics.Completeness > 0 {
			total += entityMetrics.Completeness
			count++
		}
		if entityMetrics.Consistency > 0 {
			total += entityMetrics.Consistency
			count++
		}
		if entityMetrics.Uniqueness > 0 {
			total += entityMetrics.Uniqueness
			count++
		}
		if entityMetrics.Validity > 0 {
			total += entityMetrics.Validity
			count++
		}
		if count > 0 {
			entityMetrics.OverallScore = total / float64(count)
		}

		metricsByEntity[entityType] = entityMetrics
	}

	// Рассчитываем текущий общий балл
	currentScore := 0.0
	if len(trends) > 0 {
		currentScore = trends[0].OverallScore
	} else if len(currentMetrics) > 0 {
		var total float64
		for _, metric := range currentMetrics {
			total += metric.MetricValue
		}
		currentScore = total / float64(len(currentMetrics))
	}

	dashboard := QualityDashboard{
		DatabaseID:      databaseID,
		CurrentScore:    currentScore,
		Trends:          trends,
		TopIssues:       topIssues,
		MetricsByEntity: metricsByEntity,
	}

	s.writeJSONResponse(w, dashboard, http.StatusOK)
}

// handleQualityIssues возвращает проблемы качества для базы данных
func (s *Server) handleQualityIssues(w http.ResponseWriter, r *http.Request, databaseID int) {
	// Получаем параметры фильтрации
	filters := make(map[string]interface{})

	if entityType := r.URL.Query().Get("entity_type"); entityType != "" {
		filters["entity_type"] = entityType
	}

	if severity := r.URL.Query().Get("severity"); severity != "" {
		filters["severity"] = severity
	}

	if status := r.URL.Query().Get("status"); status != "" {
		filters["status"] = status
	}

	// Получаем все выгрузки для базы данных
	uploads, err := s.db.GetAllUploads()
	if err != nil {
		s.writeJSONError(w, "Failed to get uploads", http.StatusInternalServerError)
		return
	}

	var allIssues []database.DataQualityIssue
	for _, upload := range uploads {
		if upload.DatabaseID != nil && *upload.DatabaseID == databaseID {
			issues, _, err := s.db.GetQualityIssues(upload.ID, filters, 0, 0)
			if err != nil {
				continue
			}
			allIssues = append(allIssues, issues...)
		}
	}

	s.writeJSONResponse(w, map[string]interface{}{
		"issues": allIssues,
		"total":  len(allIssues),
	}, http.StatusOK)
}

// handleQualityTrends возвращает тренды качества для базы данных
func (s *Server) handleQualityTrends(w http.ResponseWriter, r *http.Request, databaseID int) {
	days := 30
	if daysStr := r.URL.Query().Get("days"); daysStr != "" {
		if d, err := strconv.Atoi(daysStr); err == nil && d > 0 {
			days = d
		}
	}

	trends, err := s.db.GetQualityTrends(databaseID, days)
	if err != nil {
		s.writeJSONError(w, "Failed to get quality trends", http.StatusInternalServerError)
		return
	}

	s.writeJSONResponse(w, map[string]interface{}{
		"trends": trends,
		"total":  len(trends),
	}, http.StatusOK)
}

// handle1CProcessingXML генерирует актуальный XML файл обработки 1С
func (s *Server) handle1CProcessingXML(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Получаем рабочую директорию (директорию, откуда запущен сервер)
	workDir, err := os.Getwd()
	if err != nil {
		s.log(LogEntry{
			Timestamp: time.Now(),
			Level:     "ERROR",
			Message:   fmt.Sprintf("Failed to get working directory: %v", err),
			Endpoint:  "/api/1c/processing/xml",
		})
		http.Error(w, fmt.Sprintf("Failed to get working directory: %v", err), http.StatusInternalServerError)
		return
	}

	// Читаем файлы модулей с абсолютными путями
	modulePath := filepath.Join(workDir, "1c_processing", "Module", "Module.bsl")
	moduleCode, err := os.ReadFile(modulePath)
	if err != nil {
		s.log(LogEntry{
			Timestamp: time.Now(),
			Level:     "ERROR",
			Message:   fmt.Sprintf("Failed to read Module.bsl from %s: %v", modulePath, err),
			Endpoint:  "/api/1c/processing/xml",
		})
		http.Error(w, fmt.Sprintf("Failed to read module file: %v", err), http.StatusInternalServerError)
		return
	}

	extensionsPath := filepath.Join(workDir, "1c_module_extensions.bsl")
	extensionsCode, err := os.ReadFile(extensionsPath)
	if err != nil {
		// Расширения могут отсутствовать, используем пустую строку
		extensionsCode = []byte("")
		s.log(LogEntry{
			Timestamp: time.Now(),
			Level:     "WARN",
			Message:   fmt.Sprintf("Extensions file not found at %s, using empty: %v", extensionsPath, err),
			Endpoint:  "/api/1c/processing/xml",
		})
	}

	exportFunctionsPath := filepath.Join(workDir, "1c_export_functions.txt")
	exportFunctionsCode, err := os.ReadFile(exportFunctionsPath)
	if err != nil {
		// Файл может отсутствовать, используем пустую строку
		exportFunctionsCode = []byte("")
		s.log(LogEntry{
			Timestamp: time.Now(),
			Level:     "WARN",
			Message:   fmt.Sprintf("Export functions file not found, using empty: %v", err),
			Endpoint:  "/api/1c/processing/xml",
		})
	}

	// Объединяем код модуля
	fullModuleCode := string(moduleCode)

	// Добавляем код из export_functions, только область ПрограммныйИнтерфейс
	if len(exportFunctionsCode) > 0 {
		exportCodeStr := string(exportFunctionsCode)
		startMarker := "#Область ПрограммныйИнтерфейс"
		endMarker := "#КонецОбласти"

		startPos := strings.Index(exportCodeStr, startMarker)
		if startPos >= 0 {
			endPos := strings.Index(exportCodeStr[startPos+len(startMarker):], endMarker)
			if endPos >= 0 {
				endPos += startPos + len(startMarker)
				programInterfaceCode := exportCodeStr[startPos : endPos+len(endMarker)]
				fullModuleCode += "\n\n" + programInterfaceCode
			}
		}
	}

	// Добавляем расширения
	if len(extensionsCode) > 0 {
		fullModuleCode += "\n\n" + string(extensionsCode)
	}

	// Генерируем UUID для обработки
	processingUUID := strings.ToUpper(strings.ReplaceAll(uuid.New().String(), "-", ""))

	// Код формы (из Python скрипта)
	formModuleCode := `&НаКлиенте
Процедура ПриСозданииНаСервере(Отказ, СтандартнаяОбработка)
	
	// Устанавливаем значения по умолчанию
	Если Объект.АдресСервера = "" Тогда
		Объект.АдресСервера = "http://localhost:9999";
	КонецЕсли;
	
	Если Объект.РазмерПакета = 0 Тогда
		Объект.РазмерПакета = 50;
	КонецЕсли;
	
	Если Объект.ИспользоватьПакетнуюВыгрузку = Неопределено Тогда
		Объект.ИспользоватьПакетнуюВыгрузку = Истина;
	КонецЕсли;
	
КонецПроцедуры

&НаКлиенте
Процедура ПриОткрытии(Отказ)
	// Код инициализации формы
КонецПроцедуры

&НаКлиенте
Процедура ПередЗакрытием(Отказ, СтандартнаяОбработка)
	// Обработка перед закрытием формы
КонецПроцедуры`

	// Создаем единый XML файл с правильной структурой для внешней обработки 1С
	// Используем корневой элемент Configuration с ExternalDataProcessor внутри
	xmlContent := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<Configuration xmlns="http://v8.1c.ru/8.1/data/enterprise/current-config" xmlns:xs="http://www.w3.org/2001/XMLSchema" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
  <Properties>
    <SyncMode>Independent</SyncMode>
    <DataLockControlMode>Managed</DataLockControlMode>
  </Properties>
  <MetaDataObject xmlns="http://v8.1c.ru/8.1/data/enterprise" xmlns:v8="http://v8.1c.ru/8.1/data/core" xsi:type="ExternalDataProcessor">
    <Properties>
      <Name>ВыгрузкаДанныхВСервис</Name>
      <Synonym>
        <v8:item>
          <v8:lang>ru</v8:lang>
          <v8:content>Выгрузка данных в сервис нормализации</v8:content>
        </v8:item>
      </Synonym>
      <Comment>Обработка для выгрузки данных из 1С в сервис нормализации и анализа через HTTP</Comment>
      <DefaultForm>Форма</DefaultForm>
      <Help>
        <v8:item>
          <v8:lang>ru</v8:lang>
          <v8:content>Обработка для выгрузки данных</v8:content>
        </v8:item>
      </Help>
    </Properties>
    <uuid>%s</uuid>
    <module>
      <text><![CDATA[%s]]></text>
    </module>
    <forms>
      <form xsi:type="ManagedForm">
        <Properties>
          <Name>Форма</Name>
          <Synonym>
            <v8:item>
              <v8:lang>ru</v8:lang>
              <v8:content>Форма</v8:content>
            </v8:item>
          </Synonym>
        </Properties>
        <module>
          <text><![CDATA[%s]]></text>
        </module>
      </form>
    </forms>
  </MetaDataObject>
</Configuration>`, processingUUID, fullModuleCode, formModuleCode)

	// Устанавливаем заголовки для скачивания файла
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"1c_processing_%s.xml\"", time.Now().Format("20060102_150405")))
	w.WriteHeader(http.StatusOK)

	// Отправляем XML
	if _, err := w.Write([]byte(xmlContent)); err != nil {
		s.log(LogEntry{
			Timestamp: time.Now(),
			Level:     "ERROR",
			Message:   fmt.Sprintf("Failed to write XML response: %v", err),
			Endpoint:  "/api/1c/processing/xml",
		})
		return
	}

	s.log(LogEntry{
		Timestamp: time.Now(),
		Level:     "INFO",
		Message:   fmt.Sprintf("Generated 1C processing XML (UUID: %s, module size: %d chars)", processingUUID, len(fullModuleCode)),
		Endpoint:  "/api/1c/processing/xml",
	})
}

// ============================================================================
// Snapshot Handlers
// ============================================================================

// handleSnapshotsRoutes обрабатывает запросы к /api/snapshots
func (s *Server) handleSnapshotsRoutes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleListSnapshots(w, r)
	case http.MethodPost:
		s.handleCreateSnapshot(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleSnapshotRoutes обрабатывает запросы к /api/snapshots/{id} и вложенным маршрутам
func (s *Server) handleSnapshotRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/snapshots/")
	parts := strings.Split(path, "/")

	if len(parts) == 0 || parts[0] == "" {
		http.Error(w, "Snapshot ID required", http.StatusBadRequest)
		return
	}

	snapshotID, err := strconv.Atoi(parts[0])
	if err != nil {
		http.Error(w, "Invalid snapshot ID", http.StatusBadRequest)
		return
	}

	// Обработка вложенных маршрутов
	if len(parts) > 1 {
		switch parts[1] {
		case "normalize":
			if r.Method == http.MethodPost {
				s.handleNormalizeSnapshot(w, r, snapshotID)
			} else {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
			return
		case "comparison":
			if r.Method == http.MethodGet {
				s.handleSnapshotComparison(w, r, snapshotID)
			} else {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
			return
		case "metrics":
			if r.Method == http.MethodGet {
				s.handleSnapshotMetrics(w, r, snapshotID)
			} else {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
			return
		case "evolution":
			if r.Method == http.MethodGet {
				s.handleSnapshotEvolution(w, r, snapshotID)
			} else {
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
			return
		default:
			http.Error(w, "Unknown snapshot route", http.StatusNotFound)
			return
		}
	}

	// Обработка основных операций со срезом
	switch r.Method {
	case http.MethodGet:
		s.handleGetSnapshot(w, r, snapshotID)
	case http.MethodDelete:
		s.handleDeleteSnapshot(w, r, snapshotID)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleProjectSnapshotsRoutes обрабатывает запросы к /api/projects/{project_id}/snapshots
func (s *Server) handleProjectSnapshotsRoutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/projects/")
	parts := strings.Split(path, "/")

	if len(parts) < 2 || parts[0] == "" || parts[1] != "snapshots" {
		// Это не маршрут для срезов проекта, передаем дальше
		http.NotFound(w, r)
		return
	}

	projectID, err := strconv.Atoi(parts[0])
	if err != nil {
		http.Error(w, "Invalid project ID", http.StatusBadRequest)
		return
	}

	if r.Method == http.MethodGet {
		s.handleGetProjectSnapshots(w, r, projectID)
	} else {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleListSnapshots получает список всех срезов
func (s *Server) handleListSnapshots(w http.ResponseWriter, r *http.Request) {
	snapshots, err := s.db.GetAllSnapshots()
	if err != nil {
		s.writeJSONError(w, fmt.Sprintf("Failed to get snapshots: %v", err), http.StatusInternalServerError)
		return
	}

	response := SnapshotListResponse{
		Snapshots: make([]SnapshotResponse, 0, len(snapshots)),
		Total:     len(snapshots),
	}

	for _, snapshot := range snapshots {
		response.Snapshots = append(response.Snapshots, SnapshotResponse{
			ID:           snapshot.ID,
			Name:         snapshot.Name,
			Description:  snapshot.Description,
			CreatedBy:    snapshot.CreatedBy,
			CreatedAt:    snapshot.CreatedAt,
			SnapshotType: snapshot.SnapshotType,
			ProjectID:    snapshot.ProjectID,
			ClientID:     snapshot.ClientID,
		})
	}

	s.writeJSONResponse(w, response, http.StatusOK)
}

// handleCreateSnapshot создает новый срез вручную
func (s *Server) handleCreateSnapshot(w http.ResponseWriter, r *http.Request) {
	var req SnapshotRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Валидация
	if req.Name == "" {
		s.writeJSONError(w, "Name is required", http.StatusBadRequest)
		return
	}

	if req.SnapshotType == "" {
		req.SnapshotType = "manual"
	}

	// Преобразуем IncludedUploads в []database.SnapshotUpload
	var snapshotUploads []database.SnapshotUpload
	for _, u := range req.IncludedUploads {
		snapshotUploads = append(snapshotUploads, database.SnapshotUpload{
			UploadID:       u.UploadID,
			IterationLabel: u.IterationLabel,
			UploadOrder:    u.UploadOrder,
		})
	}

	// Создаем срез
	snapshot := &database.DataSnapshot{
		Name:         req.Name,
		Description:  req.Description,
		SnapshotType: req.SnapshotType,
		ProjectID:    req.ProjectID,
		ClientID:     req.ClientID,
	}

	createdSnapshot, err := s.db.CreateSnapshot(snapshot, snapshotUploads)
	if err != nil {
		s.writeJSONError(w, fmt.Sprintf("Failed to create snapshot: %v", err), http.StatusInternalServerError)
		return
	}

	response := SnapshotResponse{
		ID:           createdSnapshot.ID,
		Name:         createdSnapshot.Name,
		Description:  createdSnapshot.Description,
		CreatedBy:    createdSnapshot.CreatedBy,
		CreatedAt:    createdSnapshot.CreatedAt,
		SnapshotType: createdSnapshot.SnapshotType,
		ProjectID:    createdSnapshot.ProjectID,
		ClientID:     createdSnapshot.ClientID,
		UploadCount:  len(snapshotUploads),
	}

	s.log(LogEntry{
		Timestamp: time.Now(),
		Level:     "INFO",
		Message:   fmt.Sprintf("Created snapshot: %s (ID: %d, uploads: %d)", createdSnapshot.Name, createdSnapshot.ID, len(snapshotUploads)),
		Endpoint:  "/api/snapshots",
	})

	s.writeJSONResponse(w, response, http.StatusCreated)
}

// handleCreateAutoSnapshot создает срез автоматически по критериям
func (s *Server) handleCreateAutoSnapshot(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req AutoSnapshotRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Type != "latest_per_database" {
		s.writeJSONError(w, "Unsupported auto snapshot type", http.StatusBadRequest)
		return
	}

	if req.UploadsPerDatabase <= 0 {
		req.UploadsPerDatabase = 3 // Значение по умолчанию
	}

	createdSnapshot, err := s.createAutoSnapshot(req.ProjectID, req.UploadsPerDatabase, req.Name, req.Description)
	if err != nil {
		s.writeJSONError(w, fmt.Sprintf("Failed to create auto snapshot: %v", err), http.StatusInternalServerError)
		return
	}

	response := SnapshotResponse{
		ID:           createdSnapshot.ID,
		Name:         createdSnapshot.Name,
		Description:  createdSnapshot.Description,
		CreatedBy:    createdSnapshot.CreatedBy,
		CreatedAt:    createdSnapshot.CreatedAt,
		SnapshotType: createdSnapshot.SnapshotType,
		ProjectID:    createdSnapshot.ProjectID,
		ClientID:     createdSnapshot.ClientID,
	}

	s.log(LogEntry{
		Timestamp: time.Now(),
		Level:     "INFO",
		Message:   fmt.Sprintf("Created auto snapshot: %s (ID: %d, project: %d)", createdSnapshot.Name, createdSnapshot.ID, req.ProjectID),
		Endpoint:  "/api/snapshots/auto",
	})

	s.writeJSONResponse(w, response, http.StatusCreated)
}

// createAutoSnapshot создает срез автоматически для проекта
func (s *Server) createAutoSnapshot(projectID int, uploadsPerDatabase int, name, description string) (*database.DataSnapshot, error) {
	if s.serviceDB == nil {
		return nil, fmt.Errorf("service database not available")
	}

	// Получаем все базы данных проекта
	databases, err := s.serviceDB.GetProjectDatabases(projectID, false)
	if err != nil {
		return nil, fmt.Errorf("failed to get project databases: %w", err)
	}

	if len(databases) == 0 {
		return nil, fmt.Errorf("no databases found for project %d", projectID)
	}

	var snapshotUploads []database.SnapshotUpload
	uploadOrder := 0

	// Для каждой базы получаем N последних выгрузок
	for _, db := range databases {
		uploads, err := s.db.GetLatestUploads(db.ID, uploadsPerDatabase)
		if err != nil {
			log.Printf("Failed to get latest uploads for database %d: %v", db.ID, err)
			continue
		}

		for _, upload := range uploads {
			snapshotUploads = append(snapshotUploads, database.SnapshotUpload{
				UploadID:       upload.ID,
				IterationLabel: upload.IterationLabel,
				UploadOrder:    uploadOrder,
			})
			uploadOrder++
		}
	}

	if len(snapshotUploads) == 0 {
		return nil, fmt.Errorf("no uploads found for project %d", projectID)
	}

	// Получаем информацию о проекте для имени среза
	project, err := s.serviceDB.GetClientProject(projectID)
	if err != nil {
		return nil, fmt.Errorf("failed to get project: %w", err)
	}

	// Формируем имя среза
	if name == "" {
		name = fmt.Sprintf("Авто-срез проекта '%s' (%d выгрузок)", project.Name, len(snapshotUploads))
	}
	if description == "" {
		description = fmt.Sprintf("Автоматически созданный срез: последние %d выгрузок от каждой базы данных проекта", uploadsPerDatabase)
	}

	// Создаем срез
	snapshot := &database.DataSnapshot{
		Name:         name,
		Description:  description,
		SnapshotType: "auto_latest",
		ProjectID:    &projectID,
		ClientID:     &project.ClientID,
	}

	return s.db.CreateSnapshot(snapshot, snapshotUploads)
}

// handleGetSnapshot получает детали среза
func (s *Server) handleGetSnapshot(w http.ResponseWriter, r *http.Request, snapshotID int) {
	snapshot, uploads, err := s.db.GetSnapshotWithUploads(snapshotID)
	if err != nil {
		if err == sql.ErrNoRows {
			s.writeJSONError(w, "Snapshot not found", http.StatusNotFound)
		} else {
			s.writeJSONError(w, fmt.Sprintf("Failed to get snapshot: %v", err), http.StatusInternalServerError)
		}
		return
	}

	// Преобразуем uploads в UploadListItem
	uploadList := make([]UploadListItem, 0, len(uploads))
	for _, upload := range uploads {
		uploadList = append(uploadList, UploadListItem{
			UploadUUID:     upload.UploadUUID,
			StartedAt:      upload.StartedAt,
			CompletedAt:    upload.CompletedAt,
			Status:         upload.Status,
			Version1C:      upload.Version1C,
			ConfigName:     upload.ConfigName,
			TotalConstants: upload.TotalConstants,
			TotalCatalogs:  upload.TotalCatalogs,
			TotalItems:     upload.TotalItems,
		})
	}

	response := SnapshotResponse{
		ID:           snapshot.ID,
		Name:         snapshot.Name,
		Description:  snapshot.Description,
		CreatedBy:    snapshot.CreatedBy,
		CreatedAt:    snapshot.CreatedAt,
		SnapshotType: snapshot.SnapshotType,
		ProjectID:    snapshot.ProjectID,
		ClientID:     snapshot.ClientID,
		Uploads:      uploadList,
		UploadCount:  len(uploadList),
	}

	s.writeJSONResponse(w, response, http.StatusOK)
}

// handleGetProjectSnapshots получает все срезы проекта
func (s *Server) handleGetProjectSnapshots(w http.ResponseWriter, r *http.Request, projectID int) {
	snapshots, err := s.db.GetSnapshotsByProject(projectID)
	if err != nil {
		s.writeJSONError(w, fmt.Sprintf("Failed to get project snapshots: %v", err), http.StatusInternalServerError)
		return
	}

	response := SnapshotListResponse{
		Snapshots: make([]SnapshotResponse, 0, len(snapshots)),
		Total:     len(snapshots),
	}

	for _, snapshot := range snapshots {
		response.Snapshots = append(response.Snapshots, SnapshotResponse{
			ID:           snapshot.ID,
			Name:         snapshot.Name,
			Description:  snapshot.Description,
			CreatedBy:    snapshot.CreatedBy,
			CreatedAt:    snapshot.CreatedAt,
			SnapshotType: snapshot.SnapshotType,
			ProjectID:    snapshot.ProjectID,
			ClientID:     snapshot.ClientID,
		})
	}

	s.writeJSONResponse(w, response, http.StatusOK)
}

// handleDeleteSnapshot удаляет срез
func (s *Server) handleDeleteSnapshot(w http.ResponseWriter, r *http.Request, snapshotID int) {
	err := s.db.DeleteSnapshot(snapshotID)
	if err != nil {
		s.writeJSONError(w, fmt.Sprintf("Failed to delete snapshot: %v", err), http.StatusInternalServerError)
		return
	}

	s.log(LogEntry{
		Timestamp: time.Now(),
		Level:     "INFO",
		Message:   fmt.Sprintf("Deleted snapshot ID: %d", snapshotID),
		Endpoint:  "/api/snapshots",
	})

	s.writeJSONResponse(w, map[string]interface{}{
		"success": true,
		"message": "Snapshot deleted successfully",
	}, http.StatusOK)
}

// handleNormalizeSnapshot запускает нормализацию среза
func (s *Server) handleNormalizeSnapshot(w http.ResponseWriter, r *http.Request, snapshotID int) {
	var req SnapshotNormalizationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		// Если тело запроса пустое, используем значения по умолчанию
		req = SnapshotNormalizationRequest{
			UseAI:            false,
			MinConfidence:    0.7,
			RateLimitDelayMS: 100,
			MaxRetries:       3,
		}
	}

	result, err := s.normalizeSnapshot(snapshotID, req)
	if err != nil {
		s.log(LogEntry{
			Timestamp: time.Now(),
			Level:     "ERROR",
			Message:   fmt.Sprintf("Failed to normalize snapshot %d: %v", snapshotID, err),
			Endpoint:  "/api/snapshots/normalize",
		})
		s.writeJSONError(w, fmt.Sprintf("Normalization failed: %v", err), http.StatusInternalServerError)
		return
	}

	s.log(LogEntry{
		Timestamp: time.Now(),
		Level:     "INFO",
		Message:   fmt.Sprintf("Normalized snapshot %d: processed %d items, %d groups", snapshotID, result.TotalProcessed, result.TotalGroups),
		Endpoint:  "/api/snapshots/normalize",
	})

	s.writeJSONResponse(w, result, http.StatusOK)
}

// normalizeSnapshot выполняет сквозную нормализацию среза
func (s *Server) normalizeSnapshot(snapshotID int, req SnapshotNormalizationRequest) (*SnapshotNormalizationResult, error) {
	// Получаем срез со всеми выгрузками
	snapshot, uploads, err := s.db.GetSnapshotWithUploads(snapshotID)
	if err != nil {
		return nil, fmt.Errorf("failed to get snapshot: %w", err)
	}

	if len(uploads) == 0 {
		return nil, fmt.Errorf("snapshot has no uploads")
	}

	// Создаем нормализатор срезов
	snapshotNormalizer := normalization.NewSnapshotNormalizer()

	// Выполняем нормализацию
	result, err := snapshotNormalizer.NormalizeSnapshot(s.db, snapshot, uploads)
	if err != nil {
		return nil, fmt.Errorf("failed to normalize snapshot: %w", err)
	}

	// Сохраняем результаты нормализации для каждой выгрузки
	for uploadID, uploadResult := range result.UploadResults {
		if uploadResult.Error != "" {
			continue // Пропускаем выгрузки с ошибками
		}

		// Преобразуем NormalizedItem в map[string]interface{} для сохранения
		dataToSave := make([]map[string]interface{}, 0, len(uploadResult.NormalizedData))
		for _, item := range uploadResult.NormalizedData {
			dataToSave = append(dataToSave, map[string]interface{}{
				"source_reference":        item.SourceReference,
				"source_name":             item.SourceName,
				"code":                    item.Code,
				"normalized_name":         item.NormalizedName,
				"normalized_reference":    item.NormalizedReference,
				"category":                item.Category,
				"merged_count":            item.MergedCount,
				"source_database_id":      item.SourceDatabaseID,
				"source_iteration_number": item.SourceIterationNumber,
			})
		}

		// Сохраняем данные
		err = s.db.SaveSnapshotNormalizedDataItems(snapshotID, uploadID, dataToSave)
		if err != nil {
			s.log(LogEntry{
				Timestamp: time.Now(),
				Level:     "ERROR",
				Message:   fmt.Sprintf("Failed to save normalized data for upload %d: %v", uploadID, err),
				Endpoint:  "/api/snapshots/normalize",
			})
			// Продолжаем обработку других выгрузок
			continue
		}
	}

	// Формируем ответ
	response := &SnapshotNormalizationResult{
		SnapshotID:      result.SnapshotID,
		MasterReference: result.MasterReference,
		UploadResults:   make(map[int]*UploadNormalizationResult),
		TotalProcessed:  result.TotalProcessed,
		TotalGroups:     result.TotalGroups,
		CompletedAt:     time.Now(),
	}

	// Преобразуем результаты
	for uploadID, uploadResult := range result.UploadResults {
		var changes *NormalizationChanges
		if uploadResult.Changes != nil {
			changes = &NormalizationChanges{
				Added:   uploadResult.Changes.Added,
				Updated: uploadResult.Changes.Updated,
				Deleted: uploadResult.Changes.Deleted,
			}
		}

		response.UploadResults[uploadID] = &UploadNormalizationResult{
			UploadID:       uploadResult.UploadID,
			ProcessedCount: uploadResult.ProcessedCount,
			GroupCount:     uploadResult.GroupCount,
			Error:          uploadResult.Error,
			Changes:        changes,
		}
	}

	return response, nil
}

// handleSnapshotComparison получает сравнение итераций
func (s *Server) handleSnapshotComparison(w http.ResponseWriter, r *http.Request, snapshotID int) {
	comparison, err := s.compareSnapshotIterations(snapshotID)
	if err != nil {
		s.log(LogEntry{
			Timestamp: time.Now(),
			Level:     "ERROR",
			Message:   fmt.Sprintf("Failed to compare snapshot iterations %d: %v", snapshotID, err),
			Endpoint:  "/api/snapshots/comparison",
		})
		s.writeJSONError(w, fmt.Sprintf("Comparison failed: %v", err), http.StatusInternalServerError)
		return
	}

	s.writeJSONResponse(w, comparison, http.StatusOK)
}

// handleSnapshotMetrics получает метрики улучшения данных
func (s *Server) handleSnapshotMetrics(w http.ResponseWriter, r *http.Request, snapshotID int) {
	metrics, err := s.calculateSnapshotMetrics(snapshotID)
	if err != nil {
		s.log(LogEntry{
			Timestamp: time.Now(),
			Level:     "ERROR",
			Message:   fmt.Sprintf("Failed to calculate snapshot metrics %d: %v", snapshotID, err),
			Endpoint:  "/api/snapshots/metrics",
		})
		s.writeJSONError(w, fmt.Sprintf("Metrics calculation failed: %v", err), http.StatusInternalServerError)
		return
	}

	s.writeJSONResponse(w, metrics, http.StatusOK)
}

// handleSnapshotEvolution получает эволюцию номенклатуры
func (s *Server) handleSnapshotEvolution(w http.ResponseWriter, r *http.Request, snapshotID int) {
	evolution, err := s.getSnapshotEvolution(snapshotID)
	if err != nil {
		s.log(LogEntry{
			Timestamp: time.Now(),
			Level:     "ERROR",
			Message:   fmt.Sprintf("Failed to get snapshot evolution %d: %v", snapshotID, err),
			Endpoint:  "/api/snapshots/evolution",
		})
		s.writeJSONError(w, fmt.Sprintf("Evolution data failed: %v", err), http.StatusInternalServerError)
		return
	}

	s.writeJSONResponse(w, evolution, http.StatusOK)
}

// getModelFromConfig получает модель из WorkerConfigManager с fallback на переменные окружения
func (s *Server) getModelFromConfig() string {
	var model string
	if s.workerConfigManager != nil {
		provider, err := s.workerConfigManager.GetActiveProvider()
		if err == nil {
			activeModel, err := s.workerConfigManager.GetActiveModel(provider.Name)
			if err == nil {
				model = activeModel.Name
			} else {
				// Используем дефолтную модель из конфигурации
				config := s.workerConfigManager.GetConfig()
				if defaultModel, ok := config["default_model"].(string); ok {
					model = defaultModel
				}
			}
		}
	}

	// Fallback на переменные окружения, если WorkerConfigManager не доступен
	if model == "" {
		model = os.Getenv("ARLIAI_MODEL")
		if model == "" {
			model = "GLM-4.5-Air" // Последний fallback
		}
	}

	return model
}
