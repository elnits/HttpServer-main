package normalization

import (
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"

	"httpserver/context"
	"httpserver/nomenclature"
)

// ClassificationStep шаг классификации
type ClassificationStep struct {
	Level      KpvedLevel `json:"level"`
	LevelName  string     `json:"level_name"`
	Code       string     `json:"code"`
	Name       string     `json:"name"`
	Confidence float64    `json:"confidence"`
	Reasoning  string     `json:"reasoning"`
	Duration   int64      `json:"duration_ms"`
}

// HierarchicalResult результат иерархической классификации
type HierarchicalResult struct {
	FinalCode       string               `json:"final_code"`
	FinalName       string               `json:"final_name"`
	FinalConfidence float64              `json:"final_confidence"`
	Steps           []ClassificationStep `json:"steps"`
	TotalDuration   int64                `json:"total_duration_ms"`
	CacheHits       int                  `json:"cache_hits"`
	AICallsCount    int                  `json:"ai_calls_count"`
}

// AIResponse ответ от AI
type AIResponse struct {
	SelectedCode string  `json:"selected_code"`
	Confidence   float64 `json:"confidence"`
	Reasoning    string  `json:"reasoning"`
}

// HierarchicalClassifier иерархический классификатор КПВЭД
type HierarchicalClassifier struct {
	tree                  *KpvedTree
	db                    KpvedDB
	aiClient              *nomenclature.AIClient
	promptBuilder          *PromptBuilder
	cache                 *sync.Map
	baseWordCache         *sync.Map // кэш для корневых слов
	keywordClassifier     *KeywordClassifier
	productServiceDetector *ProductServiceDetector
	contextEnricher       *context.ContextEnricher // опциональный обогатитель контекста
	minConfidence         float64 // минимальный порог уверенности для продолжения
}

// NewHierarchicalClassifier создает новый иерархический классификатор
// Принимает KpvedDB (может быть *database.DB или *database.ServiceDB)
func NewHierarchicalClassifier(db KpvedDB, aiClient *nomenclature.AIClient) (*HierarchicalClassifier, error) {
	// Строим дерево из базы данных
	log.Printf("[HierarchicalClassifier] Building KPVED tree from database...")
	tree := NewKpvedTree()
	if err := tree.BuildFromDatabase(db); err != nil {
		log.Printf("[HierarchicalClassifier] ERROR: Failed to build KPVED tree: %v", err)
		return nil, fmt.Errorf("failed to build kpved tree: %w", err)
	}

	// Проверяем, что дерево не пустое
	nodeCount := len(tree.NodeMap)
	if nodeCount == 0 {
		log.Printf("[HierarchicalClassifier] ERROR: KPVED tree is empty!")
		return nil, fmt.Errorf("kpved tree is empty")
	}

	// Проверяем наличие секций (корневых узлов)
	sectionCount := len(tree.Root.Children)
	log.Printf("[HierarchicalClassifier] KPVED tree built successfully: %d total nodes, %d sections (root children)", nodeCount, sectionCount)

	if sectionCount == 0 {
		log.Printf("[HierarchicalClassifier] WARNING: No sections found in KPVED tree!")
	}

	classifier := &HierarchicalClassifier{
		tree:                  tree,
		db:                    db,
		aiClient:              aiClient,
		promptBuilder:          NewPromptBuilder(tree),
		cache:                 &sync.Map{},
		baseWordCache:         &sync.Map{},
		keywordClassifier:     NewKeywordClassifier(),
		productServiceDetector: NewProductServiceDetector(),
		minConfidence:         0.7, // порог 70%
	}

	log.Printf("[HierarchicalClassifier] Hierarchical classifier initialized with min confidence: %.2f", classifier.minConfidence)
	return classifier, nil
}

// SetContextEnricher устанавливает обогатитель контекста (опционально)
func (h *HierarchicalClassifier) SetContextEnricher(enricher *context.ContextEnricher) {
	h.contextEnricher = enricher
	log.Printf("[HierarchicalClassifier] Context enricher set")
}

// Classify выполняет иерархическую классификацию
func (h *HierarchicalClassifier) Classify(normalizedName, category string) (*HierarchicalResult, error) {
	startTime := time.Now()
	result := &HierarchicalResult{
		Steps: make([]ClassificationStep, 0),
	}

	// 1. Проверяем кэш для полной классификации
	cacheKey := h.getCacheKey(normalizedName, category, "")
	if cached, ok := h.cache.Load(cacheKey); ok {
		if cachedResult, ok := cached.(*HierarchicalResult); ok {
			log.Printf("[Cache] Hit for '%s' in '%s'", normalizedName, category)
			cachedResult.CacheHits++
			return cachedResult, nil
		}
	}

	// 2. Проверяем кэш корневых слов
	var rootWord string
	if h.keywordClassifier != nil {
		rootWord = h.keywordClassifier.extractRootWord(normalizedName)
		if rootWord != "" {
			baseCacheKey := rootWord + "|" + category
			if cached, ok := h.baseWordCache.Load(baseCacheKey); ok {
				if cachedResult, ok := cached.(*HierarchicalResult); ok {
					if cachedResult.FinalConfidence > 0.9 {
						log.Printf("[BaseWordCache] Hit for root word '%s' in category '%s'", rootWord, category)
						// Сохраняем в полный кэш для будущего использования
						h.cache.Store(cacheKey, cachedResult)
						return cachedResult, nil
					}
				}
			}
		}
	}

	// 3. Быстрая классификация по ключевым словам
	if h.keywordClassifier != nil {
		if keywordResult, found := h.keywordClassifier.ClassifyByKeyword(normalizedName, category); found {
			log.Printf("[Keyword] Classified '%s' as %s (%s) with confidence %.2f using keyword matching",
				normalizedName, keywordResult.FinalCode, keywordResult.FinalName, keywordResult.FinalConfidence)
			// Сохраняем в оба кэша
			h.cache.Store(cacheKey, keywordResult)
			if rootWord != "" {
				baseCacheKey := rootWord + "|" + category
				h.baseWordCache.Store(baseCacheKey, keywordResult)
			}
			keywordResult.TotalDuration = time.Since(startTime).Milliseconds()
			return keywordResult, nil
		}
	}

	// 4. Обогащаем контекст (если enricher установлен)
	if h.contextEnricher != nil {
		enrichedCtx := h.contextEnricher.Enrich(normalizedName, category)
		if enrichedCtx.ProductType != "" {
			log.Printf("[ContextEnricher] Enriched context for '%s': type=%s, confidence=%.2f",
				normalizedName, enrichedCtx.ProductType, enrichedCtx.Confidence)
			// В будущем можно использовать enrichedCtx.BuildEnhancedDescription() в промптах
		}
	}

	// 5. Определяем тип объекта (товар/услуга) перед классификацией
	var objectType string
	if h.productServiceDetector != nil {
		detectionResult := h.productServiceDetector.DetectProductOrService(normalizedName, category)
		if detectionResult.Type == ObjectTypeProduct {
			objectType = "product"
			log.Printf("[ProductServiceDetector] Detected '%s' as PRODUCT (confidence: %.2f, reasoning: %s)",
				normalizedName, detectionResult.Confidence, detectionResult.Reasoning)
		} else if detectionResult.Type == ObjectTypeService {
			objectType = "service"
			log.Printf("[ProductServiceDetector] Detected '%s' as SERVICE (confidence: %.2f, reasoning: %s)",
				normalizedName, detectionResult.Confidence, detectionResult.Reasoning)
		} else {
			log.Printf("[ProductServiceDetector] Could not determine type for '%s'", normalizedName)
		}
	}

	// Шаг 1: Классификация по секциям (A-U)
	log.Printf("[Step 1/4] Classifying '%s' by section...", normalizedName)
	sectionStep, err := h.classifyLevel(normalizedName, category, LevelSection, "", objectType)
	if err != nil {
		return nil, fmt.Errorf("section classification failed: %w", err)
	}
	result.Steps = append(result.Steps, *sectionStep)
	result.AICallsCount++

	// Проверяем уверенность
	if sectionStep.Confidence < h.minConfidence {
		log.Printf("[Stop] Low confidence at section level: %.2f", sectionStep.Confidence)
		result.FinalCode = sectionStep.Code
		result.FinalName = sectionStep.Name
		result.FinalConfidence = sectionStep.Confidence
		result.TotalDuration = time.Since(startTime).Milliseconds()
		return result, nil
	}

	// Шаг 2: Классификация по классам (01, 02, ...)
	log.Printf("[Step 2/4] Classifying '%s' by class in section %s...", normalizedName, sectionStep.Code)
	classStep, err := h.classifyLevel(normalizedName, category, LevelClass, sectionStep.Code, objectType)
	if err != nil {
		return nil, fmt.Errorf("class classification failed: %w", err)
	}
	result.Steps = append(result.Steps, *classStep)
	result.AICallsCount++

	if classStep.Confidence < h.minConfidence {
		log.Printf("[Stop] Low confidence at class level: %.2f", classStep.Confidence)
		result.FinalCode = classStep.Code
		result.FinalName = classStep.Name
		result.FinalConfidence = classStep.Confidence
		result.TotalDuration = time.Since(startTime).Milliseconds()
		return result, nil
	}

	// Шаг 3: Классификация по подклассам (XX.Y)
	log.Printf("[Step 3/4] Classifying '%s' by subclass in class %s...", normalizedName, classStep.Code)
	subclassStep, err := h.classifyLevel(normalizedName, category, LevelSubclass, classStep.Code, objectType)
	if err != nil {
		return nil, fmt.Errorf("subclass classification failed: %w", err)
	}
	result.Steps = append(result.Steps, *subclassStep)
	result.AICallsCount++

	if subclassStep.Confidence < h.minConfidence {
		log.Printf("[Stop] Low confidence at subclass level: %.2f", subclassStep.Confidence)
		result.FinalCode = subclassStep.Code
		result.FinalName = subclassStep.Name
		result.FinalConfidence = subclassStep.Confidence
		result.TotalDuration = time.Since(startTime).Milliseconds()
		return result, nil
	}

	// Шаг 4: Классификация по группам (XX.YY)
	log.Printf("[Step 4/4] Classifying '%s' by group in subclass %s...", normalizedName, subclassStep.Code)
	groupStep, err := h.classifyLevel(normalizedName, category, LevelGroup, subclassStep.Code, objectType)
	if err != nil {
		return nil, fmt.Errorf("group classification failed: %w", err)
	}
	result.Steps = append(result.Steps, *groupStep)
	result.AICallsCount++

	// Финальный результат
	result.FinalCode = groupStep.Code
	result.FinalName = groupStep.Name
	result.FinalConfidence = groupStep.Confidence
	result.TotalDuration = time.Since(startTime).Milliseconds()

	// Валидация и исправление результатов классификации
	validatedResult := h.validateAndFixClassification(result, normalizedName, category)
	if validatedResult != nil {
		result = validatedResult
		log.Printf("[Validation] Classification corrected for '%s': %s -> %s", normalizedName, groupStep.Code, result.FinalCode)
	}

	// Сохраняем в кэш
	h.cache.Store(cacheKey, result)

	// Если уверенность высокая, сохраняем также в кэш корневых слов
	if result.FinalConfidence > 0.9 && rootWord != "" && h.keywordClassifier != nil {
		baseCacheKey := rootWord + "|" + category
		h.baseWordCache.Store(baseCacheKey, result)
		// Обучаем классификатор ключевых слов на успешной классификации
		h.keywordClassifier.learnFromSuccessfulClassification(
			normalizedName, category, result.FinalCode, result.FinalName, result.FinalConfidence)
	}

	log.Printf("[Complete] Classified '%s' as %s (%s) with confidence %.2f in %dms",
		normalizedName, result.FinalCode, result.FinalName, result.FinalConfidence, result.TotalDuration)

	return result, nil
}

// classifyLevel классифицирует на указанном уровне
func (h *HierarchicalClassifier) classifyLevel(
	normalizedName, category string,
	level KpvedLevel,
	parentCode string,
	objectType string,
) (*ClassificationStep, error) {
	stepStart := time.Now()

	// Проверяем кэш для уровня
	levelCacheKey := h.getCacheKey(normalizedName, category, string(level)+":"+parentCode+":"+objectType)
	if cached, ok := h.cache.Load(levelCacheKey); ok {
		if cachedStep, ok := cached.(*ClassificationStep); ok {
			log.Printf("[Cache] Hit for level %s with parent %s", level, parentCode)
			return cachedStep, nil
		}
	}

	// Получаем кандидатов для этого уровня
	candidates := h.tree.GetNodesAtLevel(level, parentCode)
	if len(candidates) == 0 {
		log.Printf("[Level %s] ERROR: No candidates found for level %s with parent '%s'", level, level, parentCode)
		log.Printf("[Level %s] Tree stats: total nodes=%d, root children=%d", level, len(h.tree.NodeMap), len(h.tree.Root.Children))
		return nil, fmt.Errorf("no candidates found for level %s with parent %s", level, parentCode)
	}

	log.Printf("[Level %s] Found %d candidates for parent '%s'", level, len(candidates), parentCode)

	// Строим промпт с учетом типа объекта
	prompt := h.promptBuilder.BuildLevelPromptWithType(normalizedName, category, level, candidates, objectType)

	// Вызываем AI
	systemPrompt := prompt.System
	userPrompt := prompt.User

	log.Printf("[AI Call] Level: %s, Prompt size: %d bytes", level, prompt.GetPromptSize())

	response, err := h.aiClient.GetCompletion(systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("ai call failed: %w", err)
	}

	// Парсим ответ
	aiResponse, err := h.parseAIResponse(response)
	if err != nil {
		return nil, fmt.Errorf("failed to parse ai response: %w", err)
	}

	// Находим выбранный узел
	selectedNode, exists := h.tree.GetNode(aiResponse.SelectedCode)
	if !exists {
		return nil, fmt.Errorf("selected code %s not found in tree", aiResponse.SelectedCode)
	}

	// Создаем шаг
	step := &ClassificationStep{
		Level:      level,
		LevelName:  GetLevelName(level),
		Code:       selectedNode.Code,
		Name:       selectedNode.Name,
		Confidence: aiResponse.Confidence,
		Reasoning:  aiResponse.Reasoning,
		Duration:   time.Since(stepStart).Milliseconds(),
	}

	// Сохраняем в кэш
	h.cache.Store(levelCacheKey, step)

	return step, nil
}

// parseAIResponse парсит JSON ответ от AI
func (h *HierarchicalClassifier) parseAIResponse(response string) (*AIResponse, error) {
	// Убираем markdown-обертки, если есть
	response = strings.TrimSpace(response)
	response = strings.TrimPrefix(response, "```json")
	response = strings.TrimPrefix(response, "```")
	response = strings.TrimSuffix(response, "```")
	response = strings.TrimSpace(response)

	// Исправляем некорректные JSON значения (например, пустой confidence)
	// Заменяем "confidence": , на "confidence": 0.5 (значение по умолчанию)
	response = strings.ReplaceAll(response, `"confidence": ,`, `"confidence": 0.5`)
	response = strings.ReplaceAll(response, `"confidence":,`, `"confidence": 0.5`)
	response = strings.ReplaceAll(response, `"confidence": null`, `"confidence": 0.5`)
	response = strings.ReplaceAll(response, `"confidence":null`, `"confidence": 0.5`)
	// Исправляем числа, начинающиеся с точки (например, .95 -> 0.95)
	reDotNum := regexp.MustCompile(`"confidence":\s*\.([0-9]+)`)
	response = reDotNum.ReplaceAllString(response, `"confidence": 0.$1`)

	// Исправляем отсутствующие запятые между полями JSON
	// Паттерн: "confidence": 0.5\n    "reasoning" -> "confidence": 0.5,\n    "reasoning"
	// Используем регулярные выражения для более гибкой обработки
	re1 := regexp.MustCompile(`("confidence":\s*[0-9.]+)\s*\n\s*("reasoning")`)
	response = re1.ReplaceAllString(response, `$1,`+"\n    "+`$2`)
	re2 := regexp.MustCompile(`("selected_code":\s*"[^"]+")\s*\n\s*("confidence")`)
	response = re2.ReplaceAllString(response, `$1,`+"\n    "+`$2`)
	// Также обрабатываем случай без переноса строки
	re3 := regexp.MustCompile(`("confidence":\s*[0-9.]+)\s*("reasoning")`)
	response = re3.ReplaceAllString(response, `$1, $2`)
	re4 := regexp.MustCompile(`("selected_code":\s*"[^"]+")\s*("confidence")`)
	response = re4.ReplaceAllString(response, `$1, $2`)

	// Пробуем распарсить JSON
	var aiResp AIResponse
	if err := json.Unmarshal([]byte(response), &aiResp); err != nil {
		// Если парсинг не удался, пытаемся исправить JSON более агрессивно
		log.Printf("[ParseAIResponse] First parse attempt failed: %v", err)
		log.Printf("[ParseAIResponse] Original response: %s", response)

		// Более агрессивная обработка: добавляем запятые перед всеми кавычками, которые идут после значений
		// Паттерн: число или строка, затем пробелы/переносы, затем кавычка (начало нового поля)
		reFixCommas := regexp.MustCompile(`([0-9.]+|"[^"]+")\s*\n\s*(")`)
		response = reFixCommas.ReplaceAllString(response, `$1,`+"\n    "+`$2`)

		// Пробуем еще раз
		if err2 := json.Unmarshal([]byte(response), &aiResp); err2 != nil {
			return nil, fmt.Errorf("json unmarshal error: %w, response: %s", err2, response)
		}
	}

	// Валидация
	if aiResp.SelectedCode == "" {
		return nil, fmt.Errorf("empty selected_code in response")
	}

	// Если confidence не установлен или равен 0, устанавливаем значение по умолчанию
	if aiResp.Confidence == 0 {
		aiResp.Confidence = 0.5 // Значение по умолчанию
	}

	if aiResp.Confidence < 0 || aiResp.Confidence > 1 {
		// Если уверенность в процентах (0-100), конвертируем
		if aiResp.Confidence > 1 && aiResp.Confidence <= 100 {
			aiResp.Confidence = aiResp.Confidence / 100.0
		} else {
			// Если значение некорректное, используем значение по умолчанию
			log.Printf("[ParseAIResponse] Invalid confidence value: %f, using default 0.5", aiResp.Confidence)
			aiResp.Confidence = 0.5
		}
	}

	return &aiResp, nil
}

// validateAndFixClassification проверяет и исправляет явные ошибки классификации
func (h *HierarchicalClassifier) validateAndFixClassification(
	result *HierarchicalResult,
	normalizedName, category string,
) *HierarchicalResult {
	if result == nil {
		return nil
	}

	// Проверяем, является ли объект товаром
	isProduct := false
	if h.keywordClassifier != nil {
		isProduct = h.keywordClassifier.isProduct(normalizedName)
	}

	// Проверка 1: Если товар попал в категорию услуг (коды начинающиеся с 33-99)
	if isProduct && h.isServiceCode(result.FinalCode) {
		log.Printf("[Validation] ERROR: Product '%s' classified as service code %s, attempting fix...", normalizedName, result.FinalCode)
		
		// Пытаемся исправить с помощью keyword классификатора
		if h.keywordClassifier != nil {
			if keywordResult, found := h.keywordClassifier.ClassifyByKeyword(normalizedName, category); found {
				if !h.isServiceCode(keywordResult.FinalCode) {
					log.Printf("[Validation] Fixed using keyword classifier: %s -> %s", result.FinalCode, keywordResult.FinalCode)
					return keywordResult
				}
			}
		}
		
		// Если не удалось исправить, снижаем уверенность
		result.FinalConfidence *= 0.5
		log.Printf("[Validation] WARNING: Could not fix classification for '%s', reduced confidence to %.2f", normalizedName, result.FinalConfidence)
		return result
	}

	// Проверка 2: Если товар попал в "другое" (32.99.5), но есть признаки конкретного типа
	if result.FinalCode == "32.99.5" || strings.Contains(result.FinalName, "прочие") {
		if isProduct && h.keywordClassifier != nil {
			// Пытаемся найти более точную категорию через keyword классификатор
			if keywordResult, found := h.keywordClassifier.ClassifyByKeyword(normalizedName, category); found {
				if keywordResult.FinalCode != "32.99.5" && !strings.Contains(keywordResult.FinalName, "прочие") {
					log.Printf("[Validation] Fixed 'other' classification: %s -> %s", result.FinalCode, keywordResult.FinalCode)
					return keywordResult
				}
			}
		}
	}

	// Проверка 3: Специфичные ошибки классификации
	// Кабели не должны быть платами
	if strings.Contains(strings.ToLower(normalizedName), "кабель") && 
	   (result.FinalCode == "26.12.1" || strings.Contains(result.FinalName, "плат")) {
		if h.keywordClassifier != nil {
			if keywordResult, found := h.keywordClassifier.ClassifyByKeyword(normalizedName, category); found {
				if keywordResult.FinalCode == "27.32.11" || strings.Contains(keywordResult.FinalName, "кабел") {
					log.Printf("[Validation] Fixed cable misclassification: %s -> %s", result.FinalCode, keywordResult.FinalCode)
					return keywordResult
				}
			}
		}
	}

	// Датчики/преобразователи не должны быть услугами по испытаниям
	if (strings.Contains(strings.ToLower(normalizedName), "датчик") || 
		strings.Contains(strings.ToLower(normalizedName), "преобразователь")) &&
		(result.FinalCode == "71.20.1" || strings.Contains(result.FinalName, "испытан")) {
		if h.keywordClassifier != nil {
			if keywordResult, found := h.keywordClassifier.ClassifyByKeyword(normalizedName, category); found {
				if keywordResult.FinalCode == "26.51.52" || strings.Contains(keywordResult.FinalName, "прибор") {
					log.Printf("[Validation] Fixed sensor misclassification: %s -> %s", result.FinalCode, keywordResult.FinalCode)
					return keywordResult
				}
			}
		}
	}

	// Фасонные элементы не должны быть услугами
	if strings.Contains(strings.ToLower(normalizedName), "фасонные") &&
		(result.FinalCode == "96.09.1" || strings.Contains(result.FinalName, "услуг")) {
		if h.keywordClassifier != nil {
			if keywordResult, found := h.keywordClassifier.ClassifyByKeyword(normalizedName, category); found {
				if keywordResult.FinalCode == "25.11.11" || strings.Contains(keywordResult.FinalName, "конструкц") {
					log.Printf("[Validation] Fixed construction element misclassification: %s -> %s", result.FinalCode, keywordResult.FinalCode)
					return keywordResult
				}
			}
		}
	}

	return nil // Нет ошибок, возвращаем nil (не нужно исправлять)
}

// isServiceCode проверяет, является ли код КПВЭД кодом услуги
func (h *HierarchicalClassifier) isServiceCode(code string) bool {
	if len(code) < 2 {
		return false
	}
	
	// Парсим код КПВЭД (формат: XX.XX.XX или XX.XX.X)
	parts := strings.Split(code, ".")
	if len(parts) == 0 {
		return false
	}
	
	// Получаем первую часть кода (раздел/класс)
	firstPart := parts[0]
	if len(firstPart) < 2 {
		return false
	}
	
	// Пытаемся преобразовать в число
	var section int
	if _, err := fmt.Sscanf(firstPart, "%d", &section); err != nil {
		return false
	}
	
	// Разделы 33-99 - это услуги
	// Разделы 01-32 - это товары (промышленность, сельское хозяйство, строительство)
	if section >= 33 && section <= 99 {
		return true
	}
	
	// Специфичные коды услуг из примеров (даже если они в разделах 01-32)
	// Это исключения, которые нужно проверять отдельно
	serviceCodes := []string{
		"71.20.1", // Услуги по техническим испытаниям и анализу
		"71.20.11", "71.20.12", "71.20.13", "71.20.14", "71.20.19",
		"96.09.1", // Услуги индивидуальные прочие
		"96.09.11", "96.09.12", "96.09.13", "96.09.19",
	}
	
	for _, svcCode := range serviceCodes {
		if code == svcCode || strings.HasPrefix(code, svcCode+".") {
			return true
		}
	}
	
	// Проверяем по названию категории, если доступно
	// Это дополнительная проверка для надежности
	if node, exists := h.tree.GetNode(code); exists {
		nameLower := strings.ToLower(node.Name)
		serviceKeywords := []string{
			"услуг", "услуги", "работ", "работа", "обслуживание",
			"монтаж", "установка", "ремонт", "проектирование",
			"консультация", "испытание", "анализ", "исследование",
		}
		for _, keyword := range serviceKeywords {
			if strings.Contains(nameLower, keyword) {
				return true
			}
		}
	}
	
	return false
}

// getCacheKey генерирует ключ кэша
func (h *HierarchicalClassifier) getCacheKey(normalizedName, category, suffix string) string {
	if suffix != "" {
		return fmt.Sprintf("%s:%s:%s", normalizedName, category, suffix)
	}
	return fmt.Sprintf("%s:%s", normalizedName, category)
}

// ClearCache очищает кэш
func (h *HierarchicalClassifier) ClearCache() {
	h.cache = &sync.Map{}
	log.Println("[Cache] Cleared")
}

// GetCacheStats возвращает статистику кэша
func (h *HierarchicalClassifier) GetCacheStats() map[string]int {
	count := 0
	h.cache.Range(func(key, value interface{}) bool {
		count++
		return true
	})

	return map[string]int{
		"entries": count,
	}
}

// SetMinConfidence устанавливает минимальный порог уверенности
func (h *HierarchicalClassifier) SetMinConfidence(confidence float64) {
	if confidence >= 0 && confidence <= 1 {
		h.minConfidence = confidence
		log.Printf("[Config] Min confidence set to %.2f", confidence)
	}
}
