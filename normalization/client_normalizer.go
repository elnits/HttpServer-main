package normalization

import (
	"fmt"
	"log"
	"os"
	"time"

	"httpserver/database"
	"httpserver/nomenclature"
)

// ClientNormalizationResult результат нормализации для клиента
type ClientNormalizationResult struct {
	ClientID            int
	ProjectID           int
	ProcessedAt         time.Time
	TotalProcessed      int
	TotalGroups         int
	BenchmarkMatches    int
	AIEnhancedItems     int
	BasicNormalizedItems int
	NewBenchmarksCreated int
}

// ClientNormalizer нормализатор с поддержкой клиентских эталонов
type ClientNormalizer struct {
	clientID      int
	projectID     int
	db            *database.DB
	serviceDB     *database.ServiceDB
	benchmarkStore *ClientBenchmarkStore
	aiClient      *nomenclature.AIClient
	basicNormalizer *Normalizer
	events        chan<- string
}

// WorkerConfigManagerInterface интерфейс для получения конфигурации модели
type WorkerConfigManagerInterface interface {
	GetModelAndAPIKey() (apiKey string, modelName string, err error)
}

// NewClientNormalizer создает новый клиентский нормализатор
func NewClientNormalizer(clientID, projectID int, db *database.DB, serviceDB *database.ServiceDB, events chan<- string) *ClientNormalizer {
	return NewClientNormalizerWithConfig(clientID, projectID, db, serviceDB, events, nil)
}

// NewClientNormalizerWithConfig создает новый клиентский нормализатор с конфигурацией модели
func NewClientNormalizerWithConfig(clientID, projectID int, db *database.DB, serviceDB *database.ServiceDB, events chan<- string, configManager WorkerConfigManagerInterface) *ClientNormalizer {
	normalizer := &ClientNormalizer{
		clientID:       clientID,
		projectID:      projectID,
		db:             db,
		serviceDB:      serviceDB,
		benchmarkStore: NewClientBenchmarkStore(serviceDB, projectID),
		events:         events,
	}

	// Инициализация базового нормализатора
	aiConfig := &AIConfig{
		Enabled:        true,
		MinConfidence:  0.7,
		RateLimitDelay: 100 * time.Millisecond,
		MaxRetries:     3,
	}
	normalizer.basicNormalizer = NewNormalizer(db, events, aiConfig)

	// Инициализация AI клиента
	var apiKey, model string
	if configManager != nil {
		var err error
		apiKey, model, err = configManager.GetModelAndAPIKey()
		if err != nil {
			// Fallback на переменные окружения
			apiKey = os.Getenv("ARLIAI_API_KEY")
			model = os.Getenv("ARLIAI_MODEL")
		}
	} else {
		// Fallback на переменные окружения, если конфигурация не доступна
		apiKey = os.Getenv("ARLIAI_API_KEY")
		model = os.Getenv("ARLIAI_MODEL")
	}
	
	if model == "" {
		model = "gpt-4o-mini" // Последний fallback
	}
	
	if apiKey != "" {
		normalizer.aiClient = nomenclature.NewAIClient(apiKey, model)
	}

	return normalizer
}

// ProcessWithClientBenchmarks выполняет нормализацию с использованием эталонов клиента
func (c *ClientNormalizer) ProcessWithClientBenchmarks(items []*database.CatalogItem) (*ClientNormalizationResult, error) {
	result := &ClientNormalizationResult{
		ClientID:     c.clientID,
		ProjectID:    c.projectID,
		ProcessedAt:  time.Now(),
	}

	c.sendEvent("Начало нормализации с использованием эталонов клиента...")
	log.Printf("Начало нормализации для клиента %d, проекта %d", c.clientID, c.projectID)

	groups := make(map[string][]*database.CatalogItem)
	processedCount := 0

	for _, item := range items {
		// 1. Проверка против эталонов клиента
		benchmark, found := c.benchmarkStore.FindBenchmark(item.Name)
		if found {
			// Используем эталонную запись
			result.BenchmarkMatches++
			c.sendEvent(fmt.Sprintf("✓ Найдено совпадение с эталоном: %s -> %s", item.Name, benchmark.NormalizedName))
			
			// Увеличиваем счетчик использования
			if err := c.benchmarkStore.UpdateUsage(benchmark.ID); err != nil {
				log.Printf("Ошибка обновления счетчика эталона: %v", err)
			}

			// Группируем по нормализованному имени из эталона
			key := fmt.Sprintf("%s|%s", benchmark.Category, benchmark.NormalizedName)
			groups[key] = append(groups[key], item)
			processedCount++
			continue
		}

		// 2. Базовая нормализация
		category := c.basicNormalizer.categorizer.Categorize(item.Name)
		normalizedName := c.basicNormalizer.nameNormalizer.NormalizeName(item.Name)
		aiConfidence := 0.0

		// 3. AI-усиление если требуется
		if c.basicNormalizer.useAI && c.basicNormalizer.aiNormalizer != nil && 
		   c.basicNormalizer.aiNormalizer.RequiresAI(item.Name, category) {
			aiResult, err := c.basicNormalizer.processWithAI(item.Name)
			if err == nil && aiResult.Confidence >= c.basicNormalizer.aiConfig.MinConfidence {
				category = aiResult.Category
				normalizedName = aiResult.NormalizedName
				aiConfidence = aiResult.Confidence
				result.AIEnhancedItems++

				// Сохраняем как потенциальный эталон
				if aiConfidence >= 0.9 {
					if err := c.benchmarkStore.SavePotentialBenchmark(
						item.Name,
						normalizedName,
						category,
						"",
						aiConfidence,
					); err == nil {
						result.NewBenchmarksCreated++
					}
				}
			} else {
				result.BasicNormalizedItems++
			}
		} else {
			result.BasicNormalizedItems++
		}

		// Группируем записи
		key := fmt.Sprintf("%s|%s", category, normalizedName)
		groups[key] = append(groups[key], item)
		processedCount++

		// Отправляем событие каждые 1000 записей
		if processedCount%1000 == 0 {
			c.sendEvent(fmt.Sprintf("Обработано %d из %d записей", processedCount, len(items)))
		}
	}

	result.TotalProcessed = processedCount
	result.TotalGroups = len(groups)

	c.sendEvent(fmt.Sprintf("Нормализация завершена. Обработано: %d, Групп: %d, Эталонов использовано: %d", 
		result.TotalProcessed, result.TotalGroups, result.BenchmarkMatches))

	return result, nil
}

// sendEvent отправляет событие в канал
func (c *ClientNormalizer) sendEvent(message string) {
	if c.events != nil {
		select {
		case c.events <- message:
		default:
			// Канал полон, пропускаем событие
		}
	}
}

