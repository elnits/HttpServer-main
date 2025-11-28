package normalization

import (
	"regexp"
	"strings"
	"sync"
	"time"
)

// KeywordPattern паттерн для классификации по ключевому слову
type KeywordPattern struct {
	RootWord   string
	KpvedCode  string
	KpvedName  string
	Confidence float64
	Category   string // опционально, для уточнения
	Examples   []string
	MatchCount int
}

// KeywordStats статистика использования ключевого слова
type KeywordStats struct {
	TotalMatches  int
	SuccessRate   float64
	AvgConfidence float64
	LastUsed      time.Time
}

// KeywordClassifier классификатор на основе ключевых слов
type KeywordClassifier struct {
	mu         sync.RWMutex
	patterns   map[string]*KeywordPattern
	statistics map[string]*KeywordStats
}

// NewKeywordClassifier создает новый классификатор ключевых слов
func NewKeywordClassifier() *KeywordClassifier {
	kc := &KeywordClassifier{
		patterns:   make(map[string]*KeywordPattern),
		statistics: make(map[string]*KeywordStats),
	}
	kc.initializeCommonPatterns()
	return kc
}

// initializeCommonPatterns инициализирует предопределенные паттерны
func (kc *KeywordClassifier) initializeCommonPatterns() {
	kc.mu.Lock()
	defer kc.mu.Unlock()

	// Метизы и крепеж
	kc.patterns["болт"] = &KeywordPattern{
		RootWord:   "болт",
		KpvedCode:  "25.93.11",
		KpvedName:  "Болты, винты, гайки и аналогичные изделия",
		Confidence: 0.98,
		Examples:   []string{"болт м", "болт м10", "болт din"},
	}

	kc.patterns["винт"] = &KeywordPattern{
		RootWord:   "винт",
		KpvedCode:  "25.93.11",
		KpvedName:  "Болты, винты, гайки и аналогичные изделия",
		Confidence: 0.98,
	}

	kc.patterns["гайка"] = &KeywordPattern{
		RootWord:   "гайка",
		KpvedCode:  "25.93.11",
		KpvedName:  "Болты, винты, гайки и аналогичные изделия",
		Confidence: 0.98,
	}

	kc.patterns["шайба"] = &KeywordPattern{
		RootWord:   "шайба",
		KpvedCode:  "25.93.11",
		KpvedName:  "Болты, винты, гайки и аналогичные изделия",
		Confidence: 0.98,
	}

	kc.patterns["саморез"] = &KeywordPattern{
		RootWord:   "саморез",
		KpvedCode:  "25.93.11",
		KpvedName:  "Болты, винты, гайки и аналогичные изделия",
		Confidence: 0.97,
	}

	kc.patterns["заклепка"] = &KeywordPattern{
		RootWord:   "заклепка",
		KpvedCode:  "25.93.11",
		KpvedName:  "Болты, винты, гайки и аналогичные изделия",
		Confidence: 0.96,
	}

	// Инструменты
	kc.patterns["ключ"] = &KeywordPattern{
		RootWord:   "ключ",
		KpvedCode:  "25.73.11",
		KpvedName:  "Инструменты ручные",
		Confidence: 0.95,
		Examples:   []string{"ключ комбинированный", "ключ рожковый", "ключ трещоточный"},
	}

	kc.patterns["сверло"] = &KeywordPattern{
		RootWord:   "сверло",
		KpvedCode:  "25.99.12",
		KpvedName:  "Инструменты режущие",
		Confidence: 0.97,
		Examples:   []string{"сверло по металлу", "сверло к/х", "сверло с конус"},
	}

	kc.patterns["фреза"] = &KeywordPattern{
		RootWord:   "фреза",
		KpvedCode:  "25.99.12",
		KpvedName:  "Инструменты режущие",
		Confidence: 0.96,
	}

	kc.patterns["коронка"] = &KeywordPattern{
		RootWord:   "коронка",
		KpvedCode:  "25.99.12",
		KpvedName:  "Инструменты режущие",
		Confidence: 0.95,
	}

	// Подшипники
	kc.patterns["подшипник"] = &KeywordPattern{
		RootWord:   "подшипник",
		KpvedCode:  "28.15.32",
		KpvedName:  "Подшипники",
		Confidence: 0.99,
		Examples:   []string{"подшипник № 2rs", "подшипник арт", "шарикоподшипник"},
	}

	// Строительные материалы
	kc.patterns["панель"] = &KeywordPattern{
		RootWord:   "панель",
		KpvedCode:  "23.69.19",
		KpvedName:  "Изделия строительные из гипса, бетона или цемента прочие",
		Confidence: 0.90,
		Examples:   []string{"панель isowall", "панель isocop", "панель спс"},
	}

	kc.patterns["уголок"] = &KeywordPattern{
		RootWord:   "уголок",
		KpvedCode:  "24.10.73",
		KpvedName:  "Профили открытые из стали",
		Confidence: 0.96,
	}

	kc.patterns["арматура"] = &KeywordPattern{
		RootWord:   "арматура",
		KpvedCode:  "24.10.11",
		KpvedName:  "Арматура строительная",
		Confidence: 0.94,
	}

	// Электротехника
	kc.patterns["автомат"] = &KeywordPattern{
		RootWord:   "автомат",
		KpvedCode:  "27.11.21",
		KpvedName:  "Аппаратура коммутационная",
		Confidence: 0.94,
	}

	kc.patterns["кабель"] = &KeywordPattern{
		RootWord:   "кабель",
		KpvedCode:  "27.32.11",
		KpvedName:  "Кабели",
		Confidence: 0.98,
	}

	// Сантехника
	kc.patterns["муфта"] = &KeywordPattern{
		RootWord:   "муфта",
		KpvedCode:  "28.14.11",
		KpvedName:  "Арматура трубопроводная",
		Confidence: 0.95,
	}

	kc.patterns["тройник"] = &KeywordPattern{
		RootWord:   "тройник",
		KpvedCode:  "28.14.11",
		KpvedName:  "Арматура трубопроводная",
		Confidence: 0.96,
	}

	// Гидравлика и пневматика
	kc.patterns["пневмоцилиндр"] = &KeywordPattern{
		RootWord:   "пневмоцилиндр",
		KpvedCode:  "28.13.11",
		KpvedName:  "Цилиндры гидравлические и пневматические",
		Confidence: 0.98,
	}

	kc.patterns["редуктор"] = &KeywordPattern{
		RootWord:   "редуктор",
		KpvedCode:  "28.15.11",
		KpvedName:  "Редукторы",
		Confidence: 0.97,
	}

	kc.patterns["фильтр"] = &KeywordPattern{
		RootWord:   "фильтр",
		KpvedCode:  "28.29.11",
		KpvedName:  "Фильтры и аппараты для фильтрования жидкостей",
		Confidence: 0.95,
	}

	kc.patterns["сальник"] = &KeywordPattern{
		RootWord:   "сальник",
		KpvedCode:  "28.29.12",
		KpvedName:  "Уплотнения",
		Confidence: 0.96,
	}

	// Прочее
	kc.patterns["лоток"] = &KeywordPattern{
		RootWord:   "лоток",
		KpvedCode:  "25.99.19",
		KpvedName:  "Изделия металлические прочие",
		Confidence: 0.90,
	}

	// Фасонные элементы для строительных конструкций
	kc.patterns["фасонные"] = &KeywordPattern{
		RootWord:   "фасонные",
		KpvedCode:  "25.11.11",
		KpvedName:  "Конструкции и детали строительные металлические",
		Confidence: 0.95,
		Examples:   []string{"фасонные элементы", "фасонные элементы для панелей", "mq фасонные элементы"},
	}

	kc.patterns["элемент"] = &KeywordPattern{
		RootWord:   "элемент",
		KpvedCode:  "25.11.11",
		KpvedName:  "Конструкции и детали строительные металлические",
		Confidence: 0.90,
		Examples:   []string{"фасонные элементы", "соединительные элементы", "крепежные элементы"},
	}

	// Датчики и преобразователи давления
	kc.patterns["преобразователь"] = &KeywordPattern{
		RootWord:   "преобразователь",
		KpvedCode:  "26.51.52",
		KpvedName:  "Приборы и устройства для измерения или контроля давления",
		Confidence: 0.98,
		Examples:   []string{"преобразователь давления", "датчик давления", "aks преобразователь"},
	}

	kc.patterns["датчик"] = &KeywordPattern{
		RootWord:   "датчик",
		KpvedCode:  "26.51.52",
		KpvedName:  "Приборы и устройства для измерения или контроля давления",
		Confidence: 0.98,
		Examples:   []string{"датчик давления", "датчик температуры", "датчик уровня"},
	}

	// Кабели (исправление ошибки классификации как печатных плат)
	kc.patterns["контрольный"] = &KeywordPattern{
		RootWord:   "контрольный",
		KpvedCode:  "27.32.11",
		KpvedName:  "Кабели силовые",
		Confidence: 0.95,
		Examples:   []string{"контрольный кабель", "helukabel контрольный", "кабель контрольный"},
	}

	// Сэндвич-панели (металлические конструкции с утеплителем)
	kc.patterns["isowall"] = &KeywordPattern{
		RootWord:   "isowall",
		KpvedCode:  "25.11.11",
		KpvedName:  "Конструкции и детали строительные металлические",
		Confidence: 0.98,
		Examples:   []string{"панель isowall", "isowall box", "isowall fire"},
	}

	kc.patterns["сэндвич"] = &KeywordPattern{
		RootWord:   "сэндвич",
		KpvedCode:  "25.11.11",
		KpvedName:  "Конструкции и детали строительные металлические",
		Confidence: 0.98,
		Examples:   []string{"сэндвич панель", "сэндвич-панель", "панель сэндвич"},
	}

	kc.patterns["sandwich"] = &KeywordPattern{
		RootWord:   "sandwich",
		KpvedCode:  "25.11.11",
		KpvedName:  "Конструкции и детали строительные металлические",
		Confidence: 0.98,
		Examples:   []string{"sandwich panel", "sandwich панель"},
	}

	kc.patterns["isopan"] = &KeywordPattern{
		RootWord:   "isopan",
		KpvedCode:  "25.11.11",
		KpvedName:  "Конструкции и детали строительные металлические",
		Confidence: 0.95,
		Examples:   []string{"панель isopan", "isopan fire"},
	}

	kc.patterns["изопан"] = &KeywordPattern{
		RootWord:   "изопан",
		KpvedCode:  "25.11.11",
		KpvedName:  "Конструкции и детали строительные металлические",
		Confidence: 0.95,
		Examples:   []string{"панель изопан"},
	}

	// Улучшаем существующий паттерн для кабеля
	if existing, ok := kc.patterns["кабель"]; ok {
		existing.Confidence = 0.98
		existing.Examples = append(existing.Examples, "контрольный кабель", "helukabel jz-mh")
	}
}

// cleanName очищает название от артикулов, размеров, стандартов
func (kc *KeywordClassifier) cleanName(name string) string {
	// Удаляем размеры, артикулы, специальные символы
	regs := []*regexp.Regexp{
		regexp.MustCompile(`\b(арт\.?|art\.?|№)\s*[a-zA-Z0-9.-]+\b`),
		regexp.MustCompile(`\b\d+[xх]\d+`),                    // размеры типа 120x70
		regexp.MustCompile(`\b\d+[.,]\d+\b`),                 // десятичные числа
		regexp.MustCompile(`\b\d*[.,]?\d+\s*(мм|см|м|kg|кг|g|г)\b`), // единицы измерения
		regexp.MustCompile(`[^a-zA-Zа-яА-Я0-9\s]`),           // специальные символы
		regexp.MustCompile(`\b(din|iso|ral)\s*[a-zA-Z0-9]*\b`), // стандарты
		regexp.MustCompile(`\b(не использовать|not use)\b`),
	}

	cleaned := name
	for _, reg := range regs {
		cleaned = reg.ReplaceAllString(cleaned, " ")
	}

	// Удаляем лишние пробелы
	cleaned = strings.Join(strings.Fields(cleaned), " ")
	return strings.ToLower(cleaned)
}

// extractRootWord извлекает корневое слово или фразу из normalizedName
func (kc *KeywordClassifier) extractRootWord(normalizedName string) string {
	// Очищаем название
	cleanName := kc.cleanName(normalizedName)
	cleanNameLower := strings.ToLower(cleanName)

	// Сначала проверяем многословные паттерны (фразы)
	kc.mu.RLock()
	// Паттерны для фраз (в порядке приоритета - от более специфичных к общим)
	phrasePatterns := []struct {
		phrase string
		key    string
	}{
		{"сэндвич панель", "сэндвич"},
		{"сэндвич-панель", "сэндвич"},
		{"sandwich panel", "sandwich"},
		{"панель сэндвич", "сэндвич"},
		{"фасонные элементы", "фасонные"},
		{"преобразователь давления", "преобразователь"},
		{"датчик давления", "датчик"},
		{"контрольный кабель", "контрольный"},
		{"кабель контрольный", "кабель"},
	}
	
	for _, pattern := range phrasePatterns {
		if strings.Contains(cleanNameLower, pattern.phrase) {
			if _, exists := kc.patterns[pattern.key]; exists {
				kc.mu.RUnlock()
				return pattern.key
			}
		}
	}
	kc.mu.RUnlock()

	// Разбиваем на слова
	words := strings.Fields(cleanName)
	if len(words) == 0 {
		return ""
	}

	// Ищем самое длинное слово, которое есть в наших паттернах
	kc.mu.RLock()
	var candidate string
	maxLength := 0
	for _, word := range words {
		if len(word) > maxLength {
			if _, exists := kc.patterns[word]; exists {
				candidate = word
				maxLength = len(word)
			}
		}
	}
	kc.mu.RUnlock()

	if candidate != "" {
		return candidate
	}

	// Если не нашли в паттернах, берем первое слово
	return words[0]
}

// isKnownRootWord проверяет, является ли слово известным корневым словом
func (kc *KeywordClassifier) isKnownRootWord(word string) bool {
	kc.mu.RLock()
	defer kc.mu.RUnlock()

	_, exists := kc.patterns[word]
	return exists
}

// isProduct определяет, является ли объект товаром по универсальным признакам
func (kc *KeywordClassifier) isProduct(normalizedName string) bool {
	// Используем существующую функцию isLikelyProduct
	return kc.isLikelyProduct(normalizedName)
}

// detectProductType определяет тип товара по ключевым словам и контексту
func (kc *KeywordClassifier) detectProductType(normalizedName string) string {
	cleanName := strings.ToLower(kc.cleanName(normalizedName))
	
	// Сэндвич-панели (приоритетная проверка)
	if kc.containsSandwichPanel(cleanName) {
		return "sandwich_panel"
	}
	
	// Строительные материалы и элементы
	if strings.Contains(cleanName, "фасонные") || strings.Contains(cleanName, "элемент") && 
		(strings.Contains(cleanName, "панель") || strings.Contains(cleanName, "строитель") || strings.Contains(cleanName, "конструкц")) {
		return "construction"
	}
	
	// Датчики и измерительные приборы
	if strings.Contains(cleanName, "преобразователь") || strings.Contains(cleanName, "датчик") {
		if strings.Contains(cleanName, "давлен") || strings.Contains(cleanName, "температур") || 
		   strings.Contains(cleanName, "уровен") || strings.Contains(cleanName, "расход") {
			return "sensor"
		}
	}
	
	// Кабели и провода
	if strings.Contains(cleanName, "кабель") || strings.Contains(cleanName, "провод") || 
	   strings.Contains(cleanName, "контрольный") && strings.Contains(cleanName, "кабель") {
		return "cable"
	}
	
	// Метизы и крепеж
	if strings.Contains(cleanName, "болт") || strings.Contains(cleanName, "винт") || 
	   strings.Contains(cleanName, "гайка") || strings.Contains(cleanName, "саморез") {
		return "fastener"
	}
	
	// Инструменты
	if strings.Contains(cleanName, "ключ") || strings.Contains(cleanName, "сверло") || 
	   strings.Contains(cleanName, "фреза") || strings.Contains(cleanName, "инструмент") {
		return "tool"
	}
	
	// Панели и строительные материалы
	if strings.Contains(cleanName, "панель") || strings.Contains(cleanName, "профиль") {
		return "construction"
	}
	
	return "unknown"
}

// containsSandwichPanel проверяет, содержит ли название признаки сэндвич-панели
func (kc *KeywordClassifier) containsSandwichPanel(input string) bool {
	keywords := []string{
		"сэндвич панель", "сэндвич-панель", "сэндвич",
		"isowall", "sandwich panel", "sandwich",
		"isopan", "изопан", "isofire",
	}
	
	inputLower := strings.ToLower(input)
	for _, kw := range keywords {
		if strings.Contains(inputLower, kw) {
			return true
		}
	}
	return false
}

// ClassifyByKeyword пытается классифицировать по ключевому слову
func (kc *KeywordClassifier) ClassifyByKeyword(normalizedName, category string) (*HierarchicalResult, bool) {
	// Проверяем, является ли объект товаром
	if !kc.isProduct(normalizedName) {
		// Если это не товар, не используем keyword классификатор
		return nil, false
	}
	
	rootWord := kc.extractRootWord(normalizedName)
	if rootWord == "" {
		return nil, false
	}

	kc.mu.RLock()
	pattern, exists := kc.patterns[rootWord]
	kc.mu.RUnlock()

	if !exists {
		return nil, false
	}

	// Определяем тип товара для уточнения уверенности
	productType := kc.detectProductType(normalizedName)
	confidence := pattern.Confidence
	
	// Специальная обработка для сэндвич-панелей
	if productType == "sandwich_panel" {
		// Если это сэндвич-панель, но паттерн указывает на неправильную категорию, исправляем
		if rootWord == "панель" && pattern.KpvedCode != "25.11.11" {
			// Переопределяем на правильную категорию для сэндвич-панелей
			pattern.KpvedCode = "25.11.11"
			pattern.KpvedName = "Конструкции и детали строительные металлические"
			confidence = 0.98
		} else if rootWord == "isowall" || rootWord == "сэндвич" || rootWord == "sandwich" || rootWord == "isopan" {
			confidence = 0.98
		}
	}
	
	// Повышаем уверенность, если тип товара соответствует паттерну
	if productType != "unknown" && productType != "sandwich_panel" {
		// Для строительных элементов
		if productType == "construction" && (rootWord == "фасонные" || rootWord == "элемент" || rootWord == "панель") {
			// Но не для сэндвич-панелей
			if !kc.containsSandwichPanel(normalizedName) {
				confidence = 0.95
			}
		}
		// Для датчиков
		if productType == "sensor" && (rootWord == "преобразователь" || rootWord == "датчик") {
			confidence = 0.98
		}
		// Для кабелей
		if productType == "cable" && (rootWord == "кабель" || rootWord == "контрольный") {
			confidence = 0.98
		}
	}

	// Создаем результат на основе паттерна
	result := &HierarchicalResult{
		FinalCode:       pattern.KpvedCode,
		FinalName:       pattern.KpvedName,
		FinalConfidence: confidence,
		Steps: []ClassificationStep{
			{
				Level:      LevelGroup,
				LevelName:  "Keyword Match",
				Code:       pattern.KpvedCode,
				Name:       pattern.KpvedName,
				Confidence: confidence,
				Reasoning:  "Автоматическая классификация по ключевому слову '" + rootWord + "' (тип: " + productType + ")",
				Duration:   1, // Минимальное время
			},
		},
		TotalDuration: 5, // Быстрая классификация
		CacheHits:     0,
		AICallsCount:  0,
	}

	// Обновляем статистику
	kc.updateStats(rootWord, true, confidence)

	return result, true
}

// learnFromSuccessfulClassification обучается на успешной классификации
func (kc *KeywordClassifier) learnFromSuccessfulClassification(normalizedName, category, kpvedCode, kpvedName string, confidence float64) {
	if confidence < 0.8 {
		return // Только надежные классификации
	}

	rootWord := kc.extractRootWord(normalizedName)
	if rootWord == "" {
		return
	}

	kc.mu.Lock()
	defer kc.mu.Unlock()

	// Обновляем или создаем паттерн
	if pattern, exists := kc.patterns[rootWord]; exists {
		// Уточняем существующий паттерн
		pattern.MatchCount++
		if confidence > pattern.Confidence {
			pattern.Confidence = confidence
		}
		// Добавляем пример, если его нет
		found := false
		for _, ex := range pattern.Examples {
			if ex == normalizedName {
				found = true
				break
			}
		}
		if !found {
			pattern.Examples = append(pattern.Examples, normalizedName)
			if len(pattern.Examples) > 10 { // Ограничиваем количество примеров
				pattern.Examples = pattern.Examples[1:]
			}
		}
	} else {
		// Создаем новый паттерн
		kc.patterns[rootWord] = &KeywordPattern{
			RootWord:   rootWord,
			KpvedCode:  kpvedCode,
			KpvedName:  kpvedName,
			Confidence: confidence,
			Category:   category,
			Examples:   []string{normalizedName},
			MatchCount: 1,
		}
	}

	// Обновляем статистику
	kc.updateStats(rootWord, true, confidence)
}

// updateStats обновляет статистику использования ключевого слова
func (kc *KeywordClassifier) updateStats(rootWord string, success bool, confidence float64) {
	stats, exists := kc.statistics[rootWord]
	if !exists {
		stats = &KeywordStats{}
		kc.statistics[rootWord] = stats
	}

	stats.TotalMatches++
	stats.LastUsed = time.Now()

	if success {
		stats.SuccessRate = (stats.SuccessRate*float64(stats.TotalMatches-1) + 1) / float64(stats.TotalMatches)
		stats.AvgConfidence = (stats.AvgConfidence*float64(stats.TotalMatches-1) + confidence) / float64(stats.TotalMatches)
	} else {
		stats.SuccessRate = (stats.SuccessRate * float64(stats.TotalMatches-1)) / float64(stats.TotalMatches)
	}
}

// GetPatterns возвращает все паттерны (для отладки и мониторинга)
func (kc *KeywordClassifier) GetPatterns() map[string]*KeywordPattern {
	kc.mu.RLock()
	defer kc.mu.RUnlock()

	result := make(map[string]*KeywordPattern)
	for k, v := range kc.patterns {
		result[k] = v
	}
	return result
}

// GetStats возвращает статистику (для отладки и мониторинга)
func (kc *KeywordClassifier) GetStats() map[string]*KeywordStats {
	kc.mu.RLock()
	defer kc.mu.RUnlock()

	result := make(map[string]*KeywordStats)
	for k, v := range kc.statistics {
		result[k] = v
	}
	return result
}

// isLikelyProduct проверяет, является ли объект вероятно товаром по признакам
func (kc *KeywordClassifier) isLikelyProduct(input string) bool {
	// Сначала проверяем, есть ли слово в паттернах - если есть, это точно товар
	rootWord := kc.extractRootWord(input)
	if rootWord != "" {
		kc.mu.RLock()
		_, exists := kc.patterns[rootWord]
		kc.mu.RUnlock()
		if exists {
			return true
		}
	}
	
	// Признаки товара, а не услуги
	productIndicators := []string{
		"кабель", "датчик", "преобразователь", "элемент",
		"панель", "оборудование", "материал", "изделие",
		"марка", "модель", "размер", "диаметр", "длина",
		"ширина", "высота", "вес", "толщина", "артикул",
		"болт", "винт", "гайка", "шайба", "саморез",
		"муфта", "тройник", "фильтр", "редуктор",
		"подшипник", "клапан", "насос", "двигатель",
		"трансформатор", "автомат", "выключатель",
		"розетка", "вилка", "разъем", "коннектор",
		"провод", "шнур", "жгут", "лента", "пленка",
		"лист", "плита", "блок", "кирпич", "бетон",
		"цемент", "песок", "щебень", "арматура",
		"профиль", "труба", "швеллер", "уголок",
		"балка", "рейка", "доска", "брус", "бревно",
		"краска", "лак", "грунтовка", "шпаклевка",
		"герметик", "клей", "мастика", "изоляция",
		"утеплитель", "пароизоляция", "гидроизоляция",
		"фанера", "дсп", "двп", "осб", "мдф",
		"металл", "сталь", "алюминий", "медь",
		"пластик", "полиэтилен", "полипропилен",
		"резина", "силикон", "текстиль", "ткань",
		"фасонные", "комплектующие", "запчасти",
		"компонент", "деталь", "узел", "блок",
		"модуль", "система", "комплект", "набор",
		"ключ", "сверло", "фреза", "коронка", "инструмент",
	}

	inputLower := strings.ToLower(input)
	for _, indicator := range productIndicators {
		if strings.Contains(inputLower, indicator) {
			return true
		}
	}

	// Проверяем наличие технических характеристик
	hasCharacteristics := kc.hasProductCharacteristics(inputLower)
	return hasCharacteristics
}

// hasProductCharacteristics проверяет наличие технических характеристик товара
func (kc *KeywordClassifier) hasProductCharacteristics(input string) bool {
	// Паттерны технических характеристик
	characteristicPatterns := []string{
		`\b\d+\s*(мм|см|м|кг|г|л|мл|шт)\b`,                    // Размеры и единицы измерения
		`\b\d+[xх]\d+`,                                      // Размеры типа 120x70
		`\b\d+[.,]\d+\s*(мм|см|м|кг|г)\b`,                   // Десятичные размеры
		`\b(арт\.?|art\.?|№)\s*[a-zA-Z0-9.-]+\b`,            // Артикулы
		`\b(ral|din|iso|gost|гост)\s*[a-zA-Z0-9]+\b`,         // Стандарты
		`\b(марка|модель|тип|серия)\s*[:\-]?\s*[a-zA-Z0-9]+\b`, // Марки и модели
		`\b[a-zA-Z]{2,}\d+\b`,                                // Коды типа AKS32R, HELUKABEL
	}

	for _, patternStr := range characteristicPatterns {
		matched, _ := regexp.MatchString(patternStr, input)
		if matched {
			return true
		}
	}

	return false
}

