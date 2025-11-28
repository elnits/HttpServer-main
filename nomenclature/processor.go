package nomenclature

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// ProcessingStats статистика обработки
type ProcessingStats struct {
	Total      int64
	Processed  int64
	Successful int64
	Failed     int64
	StartTime  time.Time
}

// processingResult результат обработки одной записи
type processingResult struct {
	ID     int
	Result *AIProcessingResult
	Error  error
}

// NomenclatureProcessor основной процессор номенклатуры
type NomenclatureProcessor struct {
	config       Config
	db           *sql.DB
	kpved        *KpvedProcessor
	aiClient     *AIClient
	systemPrompt string
	stats        *ProcessingStats
}

// NewProcessor создает новый процессор номенклатуры
func NewProcessor(cfg Config) (*NomenclatureProcessor, error) {
	// Получаем API ключ из переменной окружения, если не указан в конфиге
	apiKey := cfg.ArliaiAPIKey
	if apiKey == "" {
		return nil, fmt.Errorf("ARLIAI_API_KEY not set in config or environment")
	}

	// Инициализируем КПВЭД процессор
	kpved := NewKpvedProcessor()
	if err := kpved.LoadKpved(cfg.KpvedFilePath); err != nil {
		return nil, fmt.Errorf("failed to load KPVED: %v", err)
	}

	// Подключаемся к БД
	db, err := sql.Open("sqlite3", cfg.DatabasePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %v", err)
	}

	processor := &NomenclatureProcessor{
		config:   cfg,
		db:       db,
		kpved:    kpved,
		aiClient: NewAIClient(apiKey, cfg.AIModel),
		stats:    &ProcessingStats{},
	}

	// Создаем системный промт
	processor.createSystemPrompt()

	return processor, nil
}

// createSystemPrompt создает системный промт с включением справочника КПВЭД
func (p *NomenclatureProcessor) createSystemPrompt() {
	kpvedData := p.kpved.GetData()
	
	// Предупреждение о длине промта
	if len(kpvedData) > 100000 {
		log.Printf("Внимание: длина справочника КПВЭД составляет %d символов, что может превысить лимит модели", len(kpvedData))
	}

	p.systemPrompt = fmt.Sprintf(`Ты - эксперт по классификации товаров по КПВЭД и нормализации наименований.

ТВОЯ ЗАДАЧА:
1. ПРОАНАЛИЗИРОВАТЬ наименование товара
2. НОРМАЛИЗОВАТЬ его (исправить опечатки, привести к стандартной форме)
3. КЛАССИФИЦИРОВАТЬ по КПВЭД используя предоставленный справочник
4. ВЕРНУТЬ результат в строго заданном JSON формате

СПРАВОЧНИК КПВЭД:
%s

ПРАВИЛА КЛАССИФИКАЦИИ:
- Используй ТОЛЬКО предоставленный справочник КПВЭД
- Выбирай наиболее специфичный (детальный) код
- Для пищевых продуктов учитывай состав и обработку
- Для промышленных товаров учитывай назначение и материал
- Если не уверен - используй более общий код

ФОРМАТ ОТВЕТА - ТОЛЬКО JSON:
{
    "normalized_name": "Нормализованное наименование",
    "kpved_code": "Код.КПВЭД", 
    "kpved_name": "Наименование группы КПВЭД",
    "confidence": 0.95
}

ВАЖНО:
- Отвечай ТОЛЬКО в указанном JSON формате
- Не добавляй никакого текста кроме JSON
- Убедись что код КПВЭД существует в справочнике
- Нормализованное наименование должно быть понятным и стандартным`, kpvedData)
}

// ProcessAll запускает обработку всех записей
func (p *NomenclatureProcessor) ProcessAll() error {
	p.stats.StartTime = time.Now()

	// Получаем общее количество записей
	var total int
	err := p.db.QueryRow(`
		SELECT COUNT(*) FROM catalog_items 
		WHERE processing_status IS NULL OR processing_status != 'completed'
	`).Scan(&total)
	if err != nil {
		return fmt.Errorf("failed to get total records: %v", err)
	}

	atomic.StoreInt64(&p.stats.Total, int64(total))
	log.Printf("Начало обработки %d записей с %d потоками", total, p.config.MaxWorkers)

	if total == 0 {
		log.Println("Нет записей для обработки")
		return nil
	}

	// Создаем каналы для работы
	jobs := make(chan int, p.config.BatchSize)
	results := make(chan processingResult, p.config.BatchSize)

	// Запускаем worker'ы
	var wg sync.WaitGroup
	for i := 0; i < p.config.MaxWorkers; i++ {
		wg.Add(1)
		go p.worker(i, jobs, results, &wg)
	}

	// Запускаем отправку заданий
	go p.jobProducer(jobs)

	// Запускаем горутину для закрытия results после завершения воркеров
	go func() {
		wg.Wait()
		close(results)
	}()

	// Обрабатываем результаты в основной горутине
	p.resultConsumer(results)

	log.Printf("Обработка завершена. Успешно: %d, Ошибки: %d",
		atomic.LoadInt64(&p.stats.Successful),
		atomic.LoadInt64(&p.stats.Failed))

	return nil
}

// jobProducer получает ID записей из БД и отправляет их в канал jobs
func (p *NomenclatureProcessor) jobProducer(jobs chan<- int) {
	defer close(jobs)

	rows, err := p.db.Query(`
		SELECT id FROM catalog_items 
		WHERE processing_status IS NULL OR processing_status != 'completed'
		ORDER BY id
	`)
	if err != nil {
		log.Printf("Ошибка при получении записей: %v", err)
		return
	}
	defer rows.Close()

	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			log.Printf("Ошибка при сканировании ID: %v", err)
			continue
		}
		jobs <- id
	}
}

// worker обрабатывает задачи из канала jobs
func (p *NomenclatureProcessor) worker(id int, jobs <-chan int, results chan<- processingResult, wg *sync.WaitGroup) {
	defer wg.Done()
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Worker %d panicked: %v", id, r)
		}
	}()

	for jobID := range jobs {
		// Rate limiting
		time.Sleep(p.config.RateLimitDelay)

		result, err := p.processSingleRecord(jobID)
		results <- processingResult{
			ID:     jobID,
			Result: result,
			Error:  err,
		}
	}
}

// processSingleRecord обрабатывает одну запись
func (p *NomenclatureProcessor) processSingleRecord(id int) (*AIProcessingResult, error) {
	// Получаем запись из БД
	var name string
	var attempts int
	err := p.db.QueryRow(`
		SELECT name, COALESCE(processing_attempts, 0) 
		FROM catalog_items WHERE id = ?
	`, id).Scan(&name, &attempts)
	if err != nil {
		return nil, fmt.Errorf("failed to get record: %v", err)
	}

	// Проверяем количество попыток
	if attempts >= p.config.MaxRetries {
		return nil, fmt.Errorf("max retries exceeded (%d)", attempts)
	}

	// Пропускаем пустые наименования
	if name == "" {
		return nil, fmt.Errorf("empty product name")
	}

	// Обрабатываем через ИИ
	result, err := p.aiClient.ProcessProduct(name, p.systemPrompt)
	if err != nil {
		return nil, fmt.Errorf("AI processing failed: %v", err)
	}

	// Валидируем код КПВЭД
	if result.KpvedCode != "" && result.KpvedCode != "unknown" {
		if !p.kpved.CodeExists(result.KpvedCode) {
			return nil, fmt.Errorf("invalid KPVED code: %s", result.KpvedCode)
		}
	}

	return result, nil
}

// resultConsumer обрабатывает результаты из канала
func (p *NomenclatureProcessor) resultConsumer(results <-chan processingResult) {
	for result := range results {
		atomic.AddInt64(&p.stats.Processed, 1)

		if result.Error != nil {
			atomic.AddInt64(&p.stats.Failed, 1)
			p.updateRecordError(result.ID, result.Error.Error())
		} else {
			atomic.AddInt64(&p.stats.Successful, 1)
			p.updateRecordSuccess(result.ID, result.Result)
		}

		// Логируем прогресс каждые 10 записей
		if atomic.LoadInt64(&p.stats.Processed)%10 == 0 {
			p.logProgress()
		}
	}
}

// updateRecordSuccess обновляет запись при успешной обработке
func (p *NomenclatureProcessor) updateRecordSuccess(id int, result *AIProcessingResult) {
	jsonData, err := json.Marshal(result)
	if err != nil {
		log.Printf("Failed to marshal result for record %d: %v", id, err)
		jsonData = []byte("{}")
	}

	_, err = p.db.Exec(`
		UPDATE catalog_items 
		SET normalized_name = ?, kpved_code = ?, kpved_name = ?,
		    processing_status = 'completed', processed_at = ?,
		    ai_response_raw = ?, processing_attempts = COALESCE(processing_attempts, 0) + 1
		WHERE id = ?
	`, result.NormalizedName, result.KpvedCode, result.KpvedName,
		time.Now().Format(time.RFC3339), string(jsonData), id)

	if err != nil {
		log.Printf("Failed to update record %d: %v", id, err)
	}
}

// updateRecordError обновляет запись при ошибке обработки
func (p *NomenclatureProcessor) updateRecordError(id int, errorMsg string) {
	_, err := p.db.Exec(`
		UPDATE catalog_items 
		SET processing_status = 'error', error_message = ?,
		    processing_attempts = COALESCE(processing_attempts, 0) + 1,
		    last_processed_at = ?
		WHERE id = ?
	`, errorMsg, time.Now().Format(time.RFC3339), id)

	if err != nil {
		log.Printf("Failed to update error for record %d: %v", id, err)
	}
}

// logProgress выводит прогресс обработки
func (p *NomenclatureProcessor) logProgress() {
	processed := atomic.LoadInt64(&p.stats.Processed)
	total := atomic.LoadInt64(&p.stats.Total)
	successful := atomic.LoadInt64(&p.stats.Successful)
	failed := atomic.LoadInt64(&p.stats.Failed)

	if total == 0 {
		return
	}

	progress := float64(processed) / float64(total) * 100
	elapsed := time.Since(p.stats.StartTime)

	log.Printf("Прогресс: %.1f%% (%d/%d) | Успешно: %d | Ошибки: %d | Время: %v",
		progress, processed, total, successful, failed, elapsed.Round(time.Second))
}

// GetStats возвращает текущую статистику
func (p *NomenclatureProcessor) GetStats() *ProcessingStats {
	return p.stats
}

// GetConfig возвращает конфигурацию процессора
func (p *NomenclatureProcessor) GetConfig() Config {
	return p.config
}

// Close закрывает подключение к БД
func (p *NomenclatureProcessor) Close() error {
	if p.db != nil {
		return p.db.Close()
	}
	return nil
}

