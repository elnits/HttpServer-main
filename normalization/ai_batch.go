package normalization

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"httpserver/nomenclature"
)

// BatchRequest представляет один элемент в батче
type BatchRequest struct {
	ID         string
	SourceName string
	ResultChan chan *BatchResult
}

// BatchResult содержит результат обработки одного элемента
type BatchResult struct {
	ID             string
	SourceName     string
	NormalizedName string
	Category       string
	Confidence     float64
	Reasoning      string
	Error          error
}

// BatchProcessorStats содержит статистику работы батч-процессора
type BatchProcessorStats struct {
	TotalBatches    int64
	TotalItems      int64
	AverageItemsPerBatch float64
	ProcessingTime  time.Duration
	LastBatchTime   time.Time
}

// BatchProcessor управляет батч-обработкой AI запросов
type BatchProcessor struct {
	aiClient       *nomenclature.AIClient
	batchSize      int
	flushInterval  time.Duration
	queue          []*BatchRequest
	mu             sync.Mutex
	stats          BatchProcessorStats
	processingChan chan struct{}
	ctx            context.Context
	cancel         context.CancelFunc
}

// NewBatchProcessor создает новый батч-процессор
func NewBatchProcessor(aiClient *nomenclature.AIClient, batchSize int, flushInterval time.Duration) *BatchProcessor {
	ctx, cancel := context.WithCancel(context.Background())

	bp := &BatchProcessor{
		aiClient:       aiClient,
		batchSize:      batchSize,
		flushInterval:  flushInterval,
		queue:          make([]*BatchRequest, 0, batchSize),
		processingChan: make(chan struct{}, 1),
		ctx:            ctx,
		cancel:         cancel,
	}

	// Запускаем горутину для периодической обработки батчей
	go bp.periodicFlush()

	return bp
}

// Add добавляет запрос в очередь
func (bp *BatchProcessor) Add(sourceName string) *BatchResult {
	resultChan := make(chan *BatchResult, 1)

	req := &BatchRequest{
		ID:         fmt.Sprintf("%d", time.Now().UnixNano()),
		SourceName: sourceName,
		ResultChan: resultChan,
	}

	bp.mu.Lock()
	bp.queue = append(bp.queue, req)
	queueSize := len(bp.queue)
	bp.mu.Unlock()

	// Если достигли размера батча, запускаем обработку
	if queueSize >= bp.batchSize {
		select {
		case bp.processingChan <- struct{}{}:
			go bp.processBatch()
		default:
			// Обработка уже идет
		}
	}

	// Ждем результат
	result := <-resultChan
	return result
}

// processBatch обрабатывает текущий батч
func (bp *BatchProcessor) processBatch() {
	bp.mu.Lock()
	if len(bp.queue) == 0 {
		bp.mu.Unlock()
		<-bp.processingChan
		return
	}

	// Берем батч из очереди
	batchSize := bp.batchSize
	if len(bp.queue) < batchSize {
		batchSize = len(bp.queue)
	}

	batch := bp.queue[:batchSize]
	bp.queue = bp.queue[batchSize:]
	bp.mu.Unlock()

	// Обрабатываем батч
	startTime := time.Now()
	results := bp.processBatchItems(batch)
	processingTime := time.Since(startTime)

	// Обновляем статистику
	bp.mu.Lock()
	bp.stats.TotalBatches++
	bp.stats.TotalItems += int64(len(batch))
	bp.stats.AverageItemsPerBatch = float64(bp.stats.TotalItems) / float64(bp.stats.TotalBatches)
	bp.stats.ProcessingTime += processingTime
	bp.stats.LastBatchTime = time.Now()
	bp.mu.Unlock()

	// Отправляем результаты обратно
	for i, result := range results {
		batch[i].ResultChan <- result
	}

	<-bp.processingChan
}

// processBatchItems обрабатывает элементы батча
func (bp *BatchProcessor) processBatchItems(batch []*BatchRequest) []*BatchResult {
	results := make([]*BatchResult, len(batch))

	// Создаем промпт для батчевой обработки
	var promptBuilder strings.Builder
	promptBuilder.WriteString("Нормализуй следующие наименования номенклатуры из 1С. ")
	promptBuilder.WriteString("Для каждого наименования определи категорию по КПВЭД и нормализуй название.\n\n")
	promptBuilder.WriteString("Верни результат в формате JSON массива:\n")
	promptBuilder.WriteString("[{\"index\": 0, \"normalized_name\": \"...\", \"category\": \"...\", \"confidence\": 0.95, \"reasoning\": \"...\"}]\n\n")
	promptBuilder.WriteString("Наименования:\n")

	for i, req := range batch {
		promptBuilder.WriteString(fmt.Sprintf("%d. %s\n", i, req.SourceName))
	}

	// Отправляем запрос к AI (используем системный промпт из AI normalizer)
	systemPrompt := "Ты - эксперт по нормализации наименований товаров. Анализируй каждый элемент и возвращай результат в формате JSON массива."
	response, err := bp.aiClient.GetCompletion(systemPrompt, promptBuilder.String())
	if err != nil {
		log.Printf("Ошибка батчевой обработки AI: %v", err)
		// В случае ошибки обрабатываем каждый элемент отдельно
		return bp.fallbackProcessing(batch)
	}

	// Парсим ответ
	batchResults, err := bp.parseBatchResponse(response)
	if err != nil {
		log.Printf("Ошибка парсинга батчевого ответа: %v", err)
		return bp.fallbackProcessing(batch)
	}

	// Сопоставляем результаты с запросами
	for i, req := range batch {
		result := &BatchResult{
			ID:         req.ID,
			SourceName: req.SourceName,
		}

		// Ищем результат по индексу
		if i < len(batchResults) {
			br := batchResults[i]
			result.NormalizedName = br.NormalizedName
			result.Category = br.Category
			result.Confidence = br.Confidence
			result.Reasoning = br.Reasoning
		} else {
			// Если результат не найден, используем fallback
			fallbackResult := bp.processSingleItem(req.SourceName)
			result.NormalizedName = fallbackResult.NormalizedName
			result.Category = fallbackResult.Category
			result.Confidence = fallbackResult.Confidence
			result.Reasoning = fallbackResult.Reasoning
		}

		results[i] = result
	}

	return results
}

// parseBatchResponse парсит ответ батчевой обработки
func (bp *BatchProcessor) parseBatchResponse(response string) ([]struct {
	Index          int     `json:"index"`
	NormalizedName string  `json:"normalized_name"`
	Category       string  `json:"category"`
	Confidence     float64 `json:"confidence"`
	Reasoning      string  `json:"reasoning"`
}, error) {
	// Ищем JSON массив в ответе
	startIdx := strings.Index(response, "[")
	endIdx := strings.LastIndex(response, "]")

	if startIdx == -1 || endIdx == -1 || startIdx >= endIdx {
		return nil, fmt.Errorf("не найден JSON массив в ответе")
	}

	jsonStr := response[startIdx : endIdx+1]

	var results []struct {
		Index          int     `json:"index"`
		NormalizedName string  `json:"normalized_name"`
		Category       string  `json:"category"`
		Confidence     float64 `json:"confidence"`
		Reasoning      string  `json:"reasoning"`
	}

	err := json.Unmarshal([]byte(jsonStr), &results)
	if err != nil {
		return nil, fmt.Errorf("ошибка парсинга JSON: %w", err)
	}

	return results, nil
}

// fallbackProcessing обрабатывает элементы по одному при ошибке батчевой обработки
func (bp *BatchProcessor) fallbackProcessing(batch []*BatchRequest) []*BatchResult {
	results := make([]*BatchResult, len(batch))

	for i, req := range batch {
		results[i] = bp.processSingleItem(req.SourceName)
		results[i].ID = req.ID
		results[i].SourceName = req.SourceName
	}

	return results
}

// processSingleItem обрабатывает один элемент
func (bp *BatchProcessor) processSingleItem(sourceName string) *BatchResult {
	systemPrompt := "Ты - эксперт по нормализации наименований товаров. Верни результат в формате JSON."
	userPrompt := fmt.Sprintf("Нормализуй наименование: %s", sourceName)
	response, err := bp.aiClient.GetCompletion(systemPrompt, userPrompt)
	if err != nil {
		return &BatchResult{
			SourceName:     sourceName,
			NormalizedName: sourceName,
			Category:       "НЕОПРЕДЕЛЕНО",
			Confidence:     0.0,
			Reasoning:      "Ошибка обработки AI",
			Error:          err,
		}
	}

	// Парсим ответ
	normalizedName, category, confidence, reasoning := bp.parseResponse(response)

	return &BatchResult{
		SourceName:     sourceName,
		NormalizedName: normalizedName,
		Category:       category,
		Confidence:     confidence,
		Reasoning:      reasoning,
	}
}

// parseResponse парсит ответ от AI (копия из ai_normalizer.go)
func (bp *BatchProcessor) parseResponse(response string) (string, string, float64, string) {
	normalizedName := ""
	category := ""
	confidence := 0.0
	reasoning := ""

	// Простой парсинг ответа
	lines := strings.Split(response, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "normalized_name:") || strings.HasPrefix(line, "Normalized Name:") {
			normalizedName = strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
		} else if strings.HasPrefix(line, "category:") || strings.HasPrefix(line, "Category:") {
			category = strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
		} else if strings.HasPrefix(line, "confidence:") || strings.HasPrefix(line, "Confidence:") {
			fmt.Sscanf(strings.TrimSpace(strings.SplitN(line, ":", 2)[1]), "%f", &confidence)
		} else if strings.HasPrefix(line, "reasoning:") || strings.HasPrefix(line, "Reasoning:") {
			reasoning = strings.TrimSpace(strings.SplitN(line, ":", 2)[1])
		}
	}

	return normalizedName, category, confidence, reasoning
}

// periodicFlush периодически обрабатывает накопленные запросы
func (bp *BatchProcessor) periodicFlush() {
	ticker := time.NewTicker(bp.flushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			bp.mu.Lock()
			queueSize := len(bp.queue)
			bp.mu.Unlock()

			if queueSize > 0 {
				select {
				case bp.processingChan <- struct{}{}:
					go bp.processBatch()
				default:
					// Обработка уже идет
				}
			}
		case <-bp.ctx.Done():
			return
		}
	}
}

// GetStats возвращает статистику работы батч-процессора
func (bp *BatchProcessor) GetStats() BatchProcessorStats {
	bp.mu.Lock()
	defer bp.mu.Unlock()
	return bp.stats
}

// Close останавливает батч-процессор
func (bp *BatchProcessor) Close() {
	bp.cancel()
}

// QueueSize возвращает текущий размер очереди
func (bp *BatchProcessor) QueueSize() int {
	bp.mu.Lock()
	defer bp.mu.Unlock()
	return len(bp.queue)
}
