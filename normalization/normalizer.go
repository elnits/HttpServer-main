package normalization

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"httpserver/database"
)

// AIConfig конфигурация для AI обработки
type AIConfig struct {
	Enabled        bool
	MinConfidence  float64
	RateLimitDelay time.Duration
	MaxRetries     int
	// Batch processing settings
	BatchEnabled      bool          // Включить батчевую обработку AI запросов
	BatchSize         int           // Размер батча (количество элементов для одновременной обработки)
	BatchFlushInterval time.Duration // Интервал автоматической обработки накопленных запросов
}

// NormalizationCheckpoint сохраненное состояние прогресса нормализации
// Позволяет возобновить обработку после сбоя
type NormalizationCheckpoint struct {
	ProcessedCount  int       `json:"processed_count"`  // Количество обработанных записей
	LastProcessedID int       `json:"last_processed_id"` // ID последней обработанной записи
	TotalCount      int       `json:"total_count"`      // Общее количество записей
	StartTime       time.Time `json:"start_time"`       // Время начала обработки
	LastSaveTime    time.Time `json:"last_save_time"`   // Время последнего сохранения checkpoint
	UploadID        int       `json:"upload_id"`        // ID выгрузки
	BatchSize       int       `json:"batch_size"`       // Размер батча
}

// Normalizer основной процессор нормализации данных
type Normalizer struct {
	db                     *database.DB
	categorizer            *Categorizer
	nameNormalizer         *NameNormalizer
	aiNormalizer           *AINormalizer
	hierarchicalClassifier *HierarchicalClassifier
	events                 chan<- string
	useAI                  bool
	aiConfig               *AIConfig
	// Конфигурация источника данных
	sourceTable     string
	referenceColumn string
	codeColumn      string
	nameColumn      string
	// Streaming и checkpoints
	enableCheckpoints bool
	checkpointDir     string
	currentCheckpoint *NormalizationCheckpoint // Текущий checkpoint для мониторинга
}

// groupKey ключ для группировки записей
type groupKey struct {
	category       string
	normalizedName string
}

// groupValue значение группы с AI и КПВЭД метаданными
type groupValue struct {
	items           []*database.CatalogItem
	aiConfidence    float64
	aiReasoning     string
	processingLevel string
	kpvedCode       string
	kpvedName       string
	kpvedConfidence float64
	attributes      map[string][]*database.ItemAttribute // code -> attributes
}

// NewNormalizer создает новый нормализатор
func NewNormalizer(db *database.DB, events chan<- string, aiConfig *AIConfig) *Normalizer {
	normalizer := &Normalizer{
		db:              db,
		categorizer:     NewCategorizer(),
		nameNormalizer:  NewNameNormalizer(),
		events:          events,
		useAI:           aiConfig != nil && aiConfig.Enabled,
		aiConfig:        aiConfig,
		// Дефолтные значения
		sourceTable:     "catalog_items",
		referenceColumn: "reference",
		codeColumn:      "code",
		nameColumn:      "name",
		// Включаем checkpoints по умолчанию
		enableCheckpoints: true,
		checkpointDir:     "./checkpoints",
	}

	// Инициализация AI нормализатора, если включен
	if normalizer.useAI {
		apiKey := os.Getenv("ARLIAI_API_KEY")
		if apiKey != "" {
			normalizer.aiNormalizer = NewAINormalizer(apiKey)
			normalizer.sendEvent("✓ AI нормализация включена")
			log.Println("AI нормализация включена")

			// Включаем батчевую обработку AI если настроена
			if normalizer.aiConfig.BatchEnabled {
				batchSize := normalizer.aiConfig.BatchSize
				if batchSize == 0 {
					batchSize = 10 // Дефолтный размер батча
				}
				flushInterval := normalizer.aiConfig.BatchFlushInterval
				if flushInterval == 0 {
					flushInterval = 5 * time.Second // Дефолтный интервал
				}
				normalizer.aiNormalizer.EnableBatchProcessing(batchSize, flushInterval)
				normalizer.sendEvent(fmt.Sprintf("✓ Батчевая обработка AI включена (размер=%d, интервал=%v)", batchSize, flushInterval))
			}

			// Инициализируем иерархический КПВЭД классификатор
			aiClient := normalizer.aiNormalizer.aiClient
			hierarchicalClassifier, err := NewHierarchicalClassifier(db, aiClient)
			if err != nil {
				log.Printf("Warning: Failed to initialize hierarchical KPVED classifier: %v", err)
				normalizer.sendEvent("⚠ КПВЭД классификатор не инициализирован")
			} else {
				normalizer.hierarchicalClassifier = hierarchicalClassifier
				normalizer.sendEvent("✓ Иерархический КПВЭД классификатор включен")
				log.Println("Иерархический КПВЭД классификатор включен")
			}
		} else {
			normalizer.sendEvent("⚠ ARLIAI_API_KEY не установлен, AI отключен")
			log.Println("ARLIAI_API_KEY не установлен, AI нормализация отключена")
			normalizer.useAI = false
		}
	}

	return normalizer
}

// SetSourceConfig устанавливает конфигурацию источника данных
func (n *Normalizer) SetSourceConfig(tableName, referenceCol, codeCol, nameCol string) {
	n.sourceTable = tableName
	n.referenceColumn = referenceCol
	n.codeColumn = codeCol
	n.nameColumn = nameCol
	log.Printf("Конфигурация источника обновлена: таблица=%s, reference=%s, code=%s, name=%s",
		tableName, referenceCol, codeCol, nameCol)
}

// SetHierarchicalClassifier устанавливает иерархический классификатор КПВЭД
func (n *Normalizer) SetHierarchicalClassifier(classifier *HierarchicalClassifier) {
	n.hierarchicalClassifier = classifier
	log.Println("Иерархический КПВЭД классификатор установлен")
}

// sendEvent отправляет событие в канал, если он доступен
func (n *Normalizer) sendEvent(message string) {
	if n.events != nil {
		select {
		case n.events <- message:
		default:
			// Канал полон, пропускаем событие
		}
	}
}

// ProcessNormalization выполняет полный процесс нормализации данных
func (n *Normalizer) ProcessNormalization() error {
	startTime := time.Now()
	n.sendEvent("Начало нормализации данных...")
	log.Printf("Начало нормализации данных...")

	// 1. Очищаем старые записи
	n.sendEvent("Очистка старых записей из catalog_items...")
	log.Printf("Очистка старых записей из catalog_items...")
	if err := n.db.CleanOldCatalogItems(); err != nil {
		n.sendEvent(fmt.Sprintf("Ошибка очистки: %v", err))
		return fmt.Errorf("failed to clean old catalog items: %w", err)
	}
	n.sendEvent("Очистка завершена")
	log.Printf("Очистка завершена")

	// 2. Получаем все записи из указанной таблицы
	n.sendEvent(fmt.Sprintf("Получение всех записей из %s...", n.sourceTable))
	log.Printf("Получение всех записей из %s (ref=%s, code=%s, name=%s)...",
		n.sourceTable, n.referenceColumn, n.codeColumn, n.nameColumn)
	items, err := n.db.GetCatalogItemsFromTable(n.sourceTable, n.referenceColumn, n.codeColumn, n.nameColumn)
	if err != nil {
		n.sendEvent(fmt.Sprintf("Ошибка получения записей: %v", err))
		return fmt.Errorf("failed to get catalog items: %w", err)
	}
	n.sendEvent(fmt.Sprintf("Получено %d записей из %s", len(items), n.sourceTable))
	log.Printf("Получено %d записей из %s", len(items), n.sourceTable)

	// CHECKPOINT: Инициализация checkpoint для отслеживания прогресса
	checkpoint := &NormalizationCheckpoint{
		ProcessedCount:  0,
		LastProcessedID: 0,
		TotalCount:      len(items),
		StartTime:       startTime,
		LastSaveTime:    startTime,
		UploadID:        1, // TODO: использовать реальный upload_id из контекста
		BatchSize:       1000,
	}
	n.currentCheckpoint = checkpoint // Сохраняем для мониторинга

	// 3. Группируем записи по (категория, normalized_name)
	n.sendEvent("Группировка записей...")
	log.Printf("Группировка записей...")

	groups := make(map[groupKey]*groupValue)
	processedCount := 0
	aiProcessedCount := 0

	for _, item := range items {
		// Базовая нормализация (правила) с извлечением атрибутов
		category := n.categorizer.Categorize(item.Name)
		normalizedName, attributes := n.nameNormalizer.ExtractAttributes(item.Name)
		if normalizedName == "" {
			normalizedName = item.Name // Используем исходное имя, если нормализация дала пустую строку
		}
		aiConfidence := 0.0
		aiReasoning := ""
		processingLevel := "basic"

		// AI обработка если требуется
		if n.useAI && n.aiNormalizer != nil && n.aiNormalizer.RequiresAI(item.Name, category) {
			aiResult, err := n.processWithAI(item.Name)
			if err != nil {
				n.sendEvent(fmt.Sprintf("⚠ AI ошибка для '%s': %v, используем правила", item.Name, err))
				log.Printf("AI ошибка для '%s': %v, используем правила", item.Name, err)
			} else if aiResult.Confidence >= n.aiConfig.MinConfidence {
				// Используем результат AI если уверенность достаточная
				category = aiResult.Category
				normalizedName = aiResult.NormalizedName
				aiConfidence = aiResult.Confidence
				aiReasoning = aiResult.Reasoning
				processingLevel = "ai_enhanced"
				aiProcessedCount++

				if aiProcessedCount%10 == 0 {
					n.sendEvent(fmt.Sprintf("🤖 AI обработано %d записей", aiProcessedCount))
				}
			} else {
				n.sendEvent(fmt.Sprintf("⚠ AI низкая уверенность (%.2f) для '%s', используем правила", aiResult.Confidence, item.Name))
			}
		}

		// Создаем ключ группы ДО КПВЭД классификации, чтобы избежать изменения ключа
		// Сначала определяем базовую категорию и нормализованное имя
		key := groupKey{category: category, normalizedName: normalizedName}
		group, exists := groups[key]

		// КПВЭД классификация выполняется ТОЛЬКО для новых групп
		// Это гарантирует, что все элементы одной группы имеют одинаковые КПВЭД коды
		kpvedCode := ""
		kpvedName := ""
		kpvedConfidence := 0.0

		if !exists {
			// Для новой группы выполняем иерархическую КПВЭД классификацию
			// Используем результат КПВЭД как категорию вместо простого Categorizer
			if n.hierarchicalClassifier != nil {
				kpvedResult, err := n.hierarchicalClassifier.Classify(normalizedName, category)
				if err != nil {
					log.Printf("Warning: Hierarchical KPVED classification failed for '%s': %v, используем простую категорию", normalizedName, err)
				} else {
					kpvedCode = kpvedResult.FinalCode
					kpvedName = kpvedResult.FinalName
					kpvedConfidence = kpvedResult.FinalConfidence
					
					// Используем название из КПВЭД как категорию, если уверенность достаточна
					// Но только если это не изменит ключ группы (чтобы не создавать дубликаты)
					if kpvedName != "" && kpvedConfidence >= 0.5 {
						// Проверяем, не создаст ли это новую группу
						newKey := groupKey{category: kpvedName, normalizedName: normalizedName}
						if _, newKeyExists := groups[newKey]; !newKeyExists {
							// Используем КПВЭД название как категорию только если это не создаст дубликат
							category = kpvedName
							key = newKey
						}
					}
					
					log.Printf("📊 KPVED (иерархический): %s -> %s (%s) [%.2f] за %dms (%d шагов, %d AI вызовов)",
						normalizedName, kpvedCode, kpvedName, kpvedConfidence,
						kpvedResult.TotalDuration, len(kpvedResult.Steps), kpvedResult.AICallsCount)
				}
			}

			// Создаем новую группу с КПВЭД данными
			group = &groupValue{
				items:           make([]*database.CatalogItem, 0),
				aiConfidence:    aiConfidence,
				aiReasoning:     aiReasoning,
				processingLevel: processingLevel,
				kpvedCode:       kpvedCode,
				kpvedName:       kpvedName,
				kpvedConfidence: kpvedConfidence,
				attributes:      make(map[string][]*database.ItemAttribute),
			}
			groups[key] = group
		} else {
			// Для существующей группы используем уже определенные КПВЭД данные
			kpvedCode = group.kpvedCode
			kpvedName = group.kpvedName
			kpvedConfidence = group.kpvedConfidence
		}

		// Добавляем запись в группу
		group.items = append(group.items, item)
		// Сохраняем атрибуты для этого элемента
		if len(attributes) > 0 {
			group.attributes[item.Code] = attributes
		}
		processedCount++

		// Отправляем событие каждые 1000 записей
		if processedCount%1000 == 0 {
			progress := float64(processedCount) / float64(len(items)) * 100
			n.sendEvent(fmt.Sprintf("Обработано %d из %d записей (%.1f%%)", processedCount, len(items), progress))
		}
	}

	groupCount := n.countGroups(groups)
	message := fmt.Sprintf("Создано %d групп", groupCount)
	if aiProcessedCount > 0 {
		message += fmt.Sprintf(" (AI улучшено: %d записей)", aiProcessedCount)
	}
	n.sendEvent(message)
	log.Print(message)

	// 4. Вставляем нормализованные данные в БД
	n.sendEvent("Вставка нормализованных данных...")
	log.Printf("Вставка нормализованных данных...")
	totalInserted := 0
	batchSize := 1000
	var batch []*database.NormalizedItem
	// Мапа для связи кода элемента с его группой (для доступа к атрибутам)
	codeToGroup := make(map[string]*groupValue)

	// Получаем StatsCollector для записи метрик
	var statsCollector *StatsCollector
	if n.aiNormalizer != nil {
		statsCollector = n.aiNormalizer.GetStatsCollector()
	}

	// Рассчитываем среднее время обработки на запись
	elapsedSoFar := time.Since(startTime)
	var avgTimePerItem time.Duration
	if len(items) > 0 {
		avgTimePerItem = elapsedSoFar / time.Duration(len(items))
	} else {
		avgTimePerItem = 0
	}

	for key, group := range groups {
		// Определяем normalized_reference = normalized_name
		normalizedReference := key.normalizedName
		mergedCount := len(group.items)

		// Для каждой записи в группе создаем нормализованную запись
		for _, item := range group.items {
			normalizedItem := &database.NormalizedItem{
				SourceReference:     item.Reference,
				SourceName:          item.Name,
				Code:                item.Code,
				NormalizedName:      key.normalizedName,
				NormalizedReference: normalizedReference,
				Category:            key.category,
				MergedCount:         mergedCount,
				AIConfidence:        group.aiConfidence,
				AIReasoning:         group.aiReasoning,
				ProcessingLevel:     group.processingLevel,
				KpvedCode:           group.kpvedCode,
				KpvedName:           group.kpvedName,
				KpvedConfidence:     group.kpvedConfidence,
			}

			batch = append(batch, normalizedItem)
			// Сохраняем связь кода с группой для доступа к атрибутам
			if item.Code != "" {
				codeToGroup[item.Code] = group
			}

			// Вставляем пакетом для производительности
			if len(batch) >= batchSize {
				// ФИЛЬТРАЦИЯ ДУБЛИКАТОВ: проверяем батч на дубликаты с данными в БД
				// Дубликаты с confidence >= 0.95 будут удалены из батча
				// Вместо вставки дубликата увеличивается merged_count существующей записи
				filteredBatch, err := n.filterDuplicatesFromBatch(batch)
				if err != nil {
					n.sendEvent(fmt.Sprintf("Ошибка фильтрации дубликатов: %v", err))
					return fmt.Errorf("failed to filter duplicates: %w", err)
				}

				// Собираем атрибуты для отфильтрованного батча
				batchAttributes := make(map[string][]*database.ItemAttribute)
				for _, normalizedItem := range filteredBatch {
					if normalizedItem.Code != "" {
						if group, ok := codeToGroup[normalizedItem.Code]; ok {
							if attrs, ok := group.attributes[normalizedItem.Code]; ok && len(attrs) > 0 {
								batchAttributes[normalizedItem.Code] = attrs
							}
						}
					}
				}

				// АТОМАРНАЯ вставка: items + attributes в ОДНОЙ транзакции
				// Если любая часть упадет - откатится ВСЕ (предотвращает частичную вставку)
				_, err = n.db.InsertNormalizedItemsWithAttributesBatch(filteredBatch, batchAttributes)
				if err != nil {
					n.sendEvent(fmt.Sprintf("Ошибка вставки пакета: %v", err))
					return fmt.Errorf("failed to insert batch: %w", err)
				}

				// Очищаем мапу для следующего батча
				codeToGroup = make(map[string]*groupValue)

				// Записываем метрики для каждой РЕАЛЬНО ВСТАВЛЕННОЙ записи (без дубликатов)
				if statsCollector != nil {
					for _, normalizedItem := range filteredBatch {
						qualityScore := normalizedItem.AIConfidence
						if qualityScore == 0.0 {
							// Для базовой нормализации используем фиксированный балл
							qualityScore = 0.5
						}
						statsCollector.RecordNormalization(
							normalizedItem.ProcessingLevel,
							qualityScore,
							avgTimePerItem,
						)
					}
				}

				totalInserted += len(filteredBatch)
				progress := float64(totalInserted) / float64(len(items)) * 100
				n.sendEvent(fmt.Sprintf("Вставлено %d записей (всего: %d, %.1f%%)", len(filteredBatch), totalInserted, progress))
				log.Printf("Вставлено %d записей (всего: %d)", len(filteredBatch), totalInserted)

				// CHECKPOINT: Сохраняем прогресс после каждого батча
				checkpoint.ProcessedCount = totalInserted
				n.currentCheckpoint = checkpoint // Обновляем для мониторинга
				if err := n.saveCheckpoint(checkpoint); err != nil {
					log.Printf("⚠ Предупреждение: не удалось сохранить checkpoint: %v", err)
				}

				batch = batch[:0] // Очищаем пакет
			}
		}
	}

	// Вставляем оставшиеся записи
	if len(batch) > 0 {
		// ФИЛЬТРАЦИЯ ДУБЛИКАТОВ для финального батча
		filteredBatch, err := n.filterDuplicatesFromBatch(batch)
		if err != nil {
			n.sendEvent(fmt.Sprintf("Ошибка фильтрации дубликатов в финальном батче: %v", err))
			return fmt.Errorf("failed to filter duplicates in final batch: %w", err)
		}

		// Собираем атрибуты для отфильтрованного финального батча
		batchAttributes := make(map[string][]*database.ItemAttribute)
		for _, normalizedItem := range filteredBatch {
			if normalizedItem.Code != "" {
				if group, ok := codeToGroup[normalizedItem.Code]; ok {
					if attrs, ok := group.attributes[normalizedItem.Code]; ok && len(attrs) > 0 {
						batchAttributes[normalizedItem.Code] = attrs
					}
				}
			}
		}

		// АТОМАРНАЯ вставка: items + attributes в ОДНОЙ транзакции
		_, err = n.db.InsertNormalizedItemsWithAttributesBatch(filteredBatch, batchAttributes)
		if err != nil {
			n.sendEvent(fmt.Sprintf("Ошибка вставки финального пакета: %v", err))
			return fmt.Errorf("failed to insert final batch: %w", err)
		}

		// Записываем метрики для РЕАЛЬНО ВСТАВЛЕННЫХ оставшихся записей
		if statsCollector != nil {
			for _, normalizedItem := range filteredBatch {
				qualityScore := normalizedItem.AIConfidence
				if qualityScore == 0.0 {
					// Для базовой нормализации используем фиксированный балл
					qualityScore = 0.5
				}
				statsCollector.RecordNormalization(
					normalizedItem.ProcessingLevel,
					qualityScore,
					avgTimePerItem,
				)
			}
		}

		totalInserted += len(filteredBatch)
		n.sendEvent(fmt.Sprintf("Вставлено %d записей (всего: %d)", len(filteredBatch), totalInserted))
		log.Printf("Вставлено %d записей (всего: %d)", len(filteredBatch), totalInserted)

		// CHECKPOINT: Сохраняем финальный прогресс
		checkpoint.ProcessedCount = totalInserted
		n.currentCheckpoint = checkpoint // Обновляем для мониторинга
		if err := n.saveCheckpoint(checkpoint); err != nil {
			log.Printf("⚠ Предупреждение: не удалось сохранить финальный checkpoint: %v", err)
		}
	}

	elapsed := time.Since(startTime)
	message = fmt.Sprintf("Нормализация завершена за %v. Всего обработано: %d записей", elapsed, totalInserted)
	n.sendEvent(message)
	log.Print(message)

	// CHECKPOINT: Удаляем checkpoint после успешного завершения
	if err := n.deleteCheckpoint(checkpoint.UploadID); err != nil {
		log.Printf("⚠ Предупреждение: не удалось удалить checkpoint: %v", err)
	} else {
		log.Printf("✓ Checkpoint успешно удален после завершения нормализации")
	}

	// Отправляем статистику AI если использовался
	if n.useAI && n.aiNormalizer != nil {
		stats := n.aiNormalizer.GetStats()
		aiMessage := fmt.Sprintf("🤖 AI Статистика: Вызовов=%d, Кэш Hit Rate=%.1f%%, Ошибок=%d, Средняя латентность=%v",
			stats.TotalCalls, stats.CacheHitRate(), stats.Errors, stats.AvgLatency())
		n.sendEvent(aiMessage)
		log.Print(aiMessage)
	}

	return nil
}

// countGroups подсчитывает общее количество групп
func (n *Normalizer) countGroups(groups map[groupKey]*groupValue) int {
	return len(groups)
}

// processWithAI обрабатывает название с помощью AI с retry logic
func (n *Normalizer) processWithAI(name string) (*AIResult, error) {
	var lastErr error
	maxRetries := n.aiConfig.MaxRetries
	if maxRetries == 0 {
		maxRetries = 3 // default
	}

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Задержка перед повторной попыткой
			time.Sleep(n.aiConfig.RateLimitDelay)
		}

		result, err := n.aiNormalizer.NormalizeWithAI(name)
		if err == nil {
			return result, nil
		}
		lastErr = err
		log.Printf("AI попытка %d/%d не удалась для '%s': %v", attempt+1, maxRetries, name, err)
	}

	return nil, fmt.Errorf("все %d попыток AI обработки не удались: %v", maxRetries, lastErr)
}

// GetAINormalizer возвращает AI нормализатор для доступа к статистике
func (n *Normalizer) GetAINormalizer() *AINormalizer {
	return n.aiNormalizer
}

// filterDuplicatesFromBatch фильтрует дубликаты из батча перед вставкой
// Проверяет наличие дубликатов с записями, которые УЖЕ в БД
// Для дубликатов с высокой уверенностью (confidence >= 0.95):
//   - Удаляет из батча
//   - Увеличивает merged_count существующей записи в БД
// Возвращает очищенный батч без дубликатов
func (n *Normalizer) filterDuplicatesFromBatch(batch []*database.NormalizedItem) ([]*database.NormalizedItem, error) {
	if len(batch) == 0 {
		return batch, nil
	}

	// 1. Собираем уникальные normalized_name из батча
	uniqueNames := make(map[string]bool)
	for _, item := range batch {
		if item.NormalizedName != "" {
			uniqueNames[item.NormalizedName] = true
		}
	}

	// Преобразуем в слайс
	names := make([]string, 0, len(uniqueNames))
	for name := range uniqueNames {
		names = append(names, name)
	}

	if len(names) == 0 {
		return batch, nil
	}

	// 2. Запрашиваем из БД существующие записи с похожими именами
	existingItems, err := n.db.GetNormalizedItemsBySimilarNames(names)
	if err != nil {
		return nil, fmt.Errorf("failed to get existing items: %w", err)
	}

	// Если в БД ничего не найдено - возвращаем батч как есть
	if len(existingItems) == 0 {
		return batch, nil
	}

	// 3. Преобразуем existing items в формат DuplicateItem для анализатора
	existingDupItems := make([]DuplicateItem, len(existingItems))
	for i, item := range existingItems {
		existingDupItems[i] = DuplicateItem{
			ID:              item.ID,
			Code:            item.Code,
			NormalizedName:  item.NormalizedName,
			Category:        item.Category,
			QualityScore:    item.QualityScore,
			MergedCount:     item.MergedCount,
			ProcessingLevel: item.ProcessingLevel,
		}
	}

	// 4. Проверяем каждый элемент батча на дубликаты с существующими записями
	analyzer := NewDuplicateAnalyzer()
	// Снижаем порог для exact matching, чтобы ловить больше дубликатов
	analyzer.exactThreshold = 0.95
	analyzer.semanticThreshold = 0.95

	// Маркируем элементы батча, которые являются дубликатами
	toRemove := make(map[int]bool) // индексы элементов батча для удаления
	duplicatesFound := 0

	for batchIdx, batchItem := range batch {
		if batchItem.NormalizedName == "" {
			continue
		}

		// Создаем DuplicateItem для элемента батча (без ID, т.к. еще не в БД)
		batchDupItem := DuplicateItem{
			ID:              -1, // Временный ID
			Code:            batchItem.Code,
			NormalizedName:  batchItem.NormalizedName,
			Category:        batchItem.Category,
			QualityScore:    0.0,
			MergedCount:     batchItem.MergedCount,
			ProcessingLevel: batchItem.ProcessingLevel,
		}

		// Проверяем на дубликаты с каждой существующей записью
		for _, existingItem := range existingDupItems {
			// Комбинируем для анализа
			testItems := []DuplicateItem{batchDupItem, existingItem}
			groups := analyzer.AnalyzeDuplicates(testItems)

			// Если найдена группа дубликатов с высокой уверенностью
			for _, group := range groups {
				if group.Confidence >= 0.95 && len(group.Items) >= 2 {
					// Проверяем, что оба элемента в группе
					hasBatch := false
					hasExisting := false
					for _, item := range group.Items {
						if item.ID == -1 {
							hasBatch = true
						} else {
							hasExisting = true
						}
					}

					if hasBatch && hasExisting {
						// Нашли дубликат! Помечаем элемент батча на удаление
						toRemove[batchIdx] = true
						duplicatesFound++

						// Увеличиваем merged_count существующей записи
						err := n.db.IncrementMergedCount(existingItem.ID)
						if err != nil {
							log.Printf("ПРЕДУПРЕЖДЕНИЕ: не удалось увеличить merged_count для записи %d: %v", existingItem.ID, err)
						} else {
							log.Printf("Найден дубликат: '%s' (батч) совпадает с записью ID=%d в БД (confidence=%.2f). Merged_count увеличен.", batchItem.NormalizedName, existingItem.ID, group.Confidence)
						}

						// Прерываем внутренний цикл, т.к. дубликат уже найден
						break
					}
				}
			}

			// Если элемент уже помечен на удаление, не проверяем дальше
			if toRemove[batchIdx] {
				break
			}
		}
	}

	// 5. Фильтруем батч, удаляя дубликаты
	if len(toRemove) == 0 {
		return batch, nil
	}

	filtered := make([]*database.NormalizedItem, 0, len(batch)-len(toRemove))
	for i, item := range batch {
		if !toRemove[i] {
			filtered = append(filtered, item)
		}
	}

	log.Printf("Фильтрация дубликатов: найдено %d дубликатов, удалено из батча. Осталось %d записей для вставки.", duplicatesFound, len(filtered))
	n.sendEvent(fmt.Sprintf("Найдено и отфильтровано %d дубликатов", duplicatesFound))

	return filtered, nil
}

// --- Методы для работы с checkpoints ---

// getCheckpointPath возвращает путь к файлу checkpoint для указанного uploadID
func (n *Normalizer) getCheckpointPath(uploadID int) string {
	return filepath.Join(n.checkpointDir, fmt.Sprintf("checkpoint_%d.json", uploadID))
}

// saveCheckpoint сохраняет текущее состояние прогресса в checkpoint файл
func (n *Normalizer) saveCheckpoint(checkpoint *NormalizationCheckpoint) error {
	if !n.enableCheckpoints {
		return nil
	}

	// Создаем директорию для checkpoints, если её нет
	if err := os.MkdirAll(n.checkpointDir, 0755); err != nil {
		return fmt.Errorf("failed to create checkpoint directory: %w", err)
	}

	checkpoint.LastSaveTime = time.Now()

	// Сериализуем checkpoint в JSON
	data, err := json.MarshalIndent(checkpoint, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal checkpoint: %w", err)
	}

	// Сохраняем в файл
	checkpointPath := n.getCheckpointPath(checkpoint.UploadID)
	if err := os.WriteFile(checkpointPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write checkpoint file: %w", err)
	}

	log.Printf("Checkpoint сохранен: %d/%d записей обработано (%.1f%%)",
		checkpoint.ProcessedCount, checkpoint.TotalCount,
		float64(checkpoint.ProcessedCount)/float64(checkpoint.TotalCount)*100)

	return nil
}

// loadCheckpoint загружает сохраненное состояние из checkpoint файла
// Возвращает nil, если checkpoint не найден (начинаем с начала)
func (n *Normalizer) loadCheckpoint(uploadID int) (*NormalizationCheckpoint, error) {
	if !n.enableCheckpoints {
		return nil, nil
	}

	checkpointPath := n.getCheckpointPath(uploadID)

	// Проверяем существование файла
	if _, err := os.Stat(checkpointPath); os.IsNotExist(err) {
		return nil, nil // Checkpoint не найден - начинаем с начала
	}

	// Читаем файл
	data, err := os.ReadFile(checkpointPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read checkpoint file: %w", err)
	}

	// Десериализуем из JSON
	var checkpoint NormalizationCheckpoint
	if err := json.Unmarshal(data, &checkpoint); err != nil {
		return nil, fmt.Errorf("failed to unmarshal checkpoint: %w", err)
	}

	log.Printf("Checkpoint загружен: восстановление с позиции %d/%d (%.1f%%)",
		checkpoint.ProcessedCount, checkpoint.TotalCount,
		float64(checkpoint.ProcessedCount)/float64(checkpoint.TotalCount)*100)
	n.sendEvent(fmt.Sprintf("Восстановление с checkpoint: %d/%d записей", checkpoint.ProcessedCount, checkpoint.TotalCount))

	return &checkpoint, nil
}

// deleteCheckpoint удаляет checkpoint файл после успешного завершения обработки
func (n *Normalizer) deleteCheckpoint(uploadID int) error {
	if !n.enableCheckpoints {
		return nil
	}

	checkpointPath := n.getCheckpointPath(uploadID)

	if err := os.Remove(checkpointPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete checkpoint file: %w", err)
	}

	log.Printf("Checkpoint удален после успешного завершения обработки")
	return nil
}

// GetCheckpointStatus возвращает текущий статус checkpoint для мониторинга
func (n *Normalizer) GetCheckpointStatus() map[string]interface{} {
	if !n.enableCheckpoints {
		return map[string]interface{}{
			"enabled":          false,
			"active":           false,
			"processed_count":  0,
			"total_count":      0,
			"progress_percent": 0.0,
		}
	}

	if n.currentCheckpoint == nil {
		return map[string]interface{}{
			"enabled":          true,
			"active":           false,
			"processed_count":  0,
			"total_count":      0,
			"progress_percent": 0.0,
		}
	}

	// Проверяем, активен ли процесс нормализации
	// Если прошло более 5 минут с последнего сохранения, считаем процесс неактивным
	active := false
	if !n.currentCheckpoint.LastSaveTime.IsZero() {
		timeSinceLastSave := time.Since(n.currentCheckpoint.LastSaveTime)
		active = timeSinceLastSave < 5*time.Minute && n.currentCheckpoint.ProcessedCount < n.currentCheckpoint.TotalCount
	}

	progressPercent := 0.0
	if n.currentCheckpoint.TotalCount > 0 {
		progressPercent = float64(n.currentCheckpoint.ProcessedCount) / float64(n.currentCheckpoint.TotalCount) * 100.0
	}

	var lastCheckpointTime *string
	if !n.currentCheckpoint.LastSaveTime.IsZero() {
		timeStr := n.currentCheckpoint.LastSaveTime.Format(time.RFC3339)
		lastCheckpointTime = &timeStr
	}

	var currentBatchID *string
	if n.currentCheckpoint.UploadID > 0 {
		batchID := fmt.Sprintf("upload_%d", n.currentCheckpoint.UploadID)
		currentBatchID = &batchID
	}

	return map[string]interface{}{
		"enabled":           true,
		"active":            active,
		"processed_count":   n.currentCheckpoint.ProcessedCount,
		"total_count":       n.currentCheckpoint.TotalCount,
		"progress_percent":  progressPercent,
		"last_checkpoint_time": lastCheckpointTime,
		"current_batch_id":    currentBatchID,
	}
}

