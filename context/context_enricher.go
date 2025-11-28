package context

import (
	"database/sql"
	"strings"
	"sync"
)

// EnrichedContext содержит дополнительную информацию о товаре
type EnrichedContext struct {
	NormalizedName string
	Category       string
	Description    string
	TechnicalSpecs map[string]string
	Keywords       []string
	ProductType    string
	Source         string
	Confidence     float64
}

// ContextEnricher обогащает контекст товара
type ContextEnricher struct {
	db    *sql.DB
	cache map[string]EnrichedContext
	mu    sync.RWMutex
	knowledge *ConstructionKnowledge
}

// NewContextEnricher создает новый обогатитель контекста
func NewContextEnricher(db *sql.DB) *ContextEnricher {
	return &ContextEnricher{
		db:        db,
		cache:     make(map[string]EnrichedContext),
		knowledge: NewConstructionKnowledge(),
	}
}

// Enrich собирает дополнительный контекст для товара
func (ce *ContextEnricher) Enrich(normalizedName, category string) EnrichedContext {
	cacheKey := normalizedName + "|" + category

	// Проверяем кэш
	ce.mu.RLock()
	if ctx, exists := ce.cache[cacheKey]; exists {
		ce.mu.RUnlock()
		return ctx
	}
	ce.mu.RUnlock()

	// Собираем контекст из различных источников
	ctx := EnrichedContext{
		NormalizedName: normalizedName,
		Category:       category,
		TechnicalSpecs: make(map[string]string),
		Keywords:       []string{},
		Confidence:     0.5,
	}

	// 1. Извлечение ключевых слов из названия
	ctx = ce.extractKeywords(ctx)

	// 2. Определение типа продукта на основе паттернов
	ctx = ce.determineProductType(ctx)

	// 3. Добавление технических характеристик из известных паттернов
	ctx = ce.addTechnicalSpecs(ctx)

	// 4. Поиск в базе данных похожих товаров (если есть доступ к БД)
	if ce.db != nil {
		ctx = ce.enrichFromDatabase(ctx)
	}

	// Сохраняем в кэш
	ce.mu.Lock()
	ce.cache[cacheKey] = ctx
	ce.mu.Unlock()

	return ctx
}

// enrichFromDatabase ищет похожие товары в базе данных
func (ce *ContextEnricher) enrichFromDatabase(ctx EnrichedContext) EnrichedContext {
	if ce.db == nil {
		return ctx
	}

	// Извлекаем основные ключевые слова для поиска
	mainKeywords := extractMainKeywords(ctx.NormalizedName)
	if mainKeywords == "" {
		return ctx
	}

	query := `
		SELECT normalized_name, category, description 
		FROM normalized_data 
		WHERE (normalized_name LIKE ? OR normalized_name LIKE ?)
		AND category != 'другое'
		AND kpved_code IS NOT NULL
		LIMIT 5
	`

	searchTerm1 := "%" + mainKeywords + "%"
	searchTerm2 := "%" + ctx.NormalizedName + "%"

	rows, err := ce.db.Query(query, searchTerm1, searchTerm2)
	if err != nil {
		return ctx
	}
	defer rows.Close()

	var similarDescriptions []string
	for rows.Next() {
		var name, category, desc string
		if err := rows.Scan(&name, &category, &desc); err == nil && desc != "" {
			similarDescriptions = append(similarDescriptions, desc)
		}
	}

	if len(similarDescriptions) > 0 {
		ctx.Description = strings.Join(similarDescriptions, ". ")
		ctx.Source = "database_similar"
		ctx.Confidence = 0.7
	}

	return ctx
}

// extractKeywords извлекает ключевые слова из названия
func (ce *ContextEnricher) extractKeywords(ctx EnrichedContext) EnrichedContext {
	name := strings.ToLower(ctx.NormalizedName)

	// Используем базу знаний для извлечения ключевых слов
	if ce.knowledge != nil {
		for pattern, meaning := range ce.knowledge.MaterialPatterns {
			if strings.Contains(name, pattern) {
				ctx.Keywords = append(ctx.Keywords, meaning)
			}
		}
	}

	// Дополнительные ключевые слова для строительных материалов
	constructionKeywords := map[string]string{
		"панель":   "строительная_панель",
		"fire":     "огнестойкий",
		"box":      "конструкция",
		"wall":     "стеновой",
		"изовол":   "минеральная_вата",
		"минераль": "минеральная_вата",
	}

	for keyword, meaning := range constructionKeywords {
		if strings.Contains(name, keyword) {
			// Проверяем, что это ключевое слово еще не добавлено
			found := false
			for _, kw := range ctx.Keywords {
				if kw == meaning {
					found = true
					break
				}
			}
			if !found {
				ctx.Keywords = append(ctx.Keywords, meaning)
			}
		}
	}

	return ctx
}

// determineProductType определяет тип продукта
func (ce *ContextEnricher) determineProductType(ctx EnrichedContext) EnrichedContext {
	name := strings.ToLower(ctx.NormalizedName)

	// Используем базу знаний для определения типа
	if ce.knowledge != nil {
		for pattern, productType := range ce.knowledge.MaterialPatterns {
			if strings.Contains(name, pattern) {
				if productType == "сэндвич_панель" {
					ctx.ProductType = "сэндвич_панель"
					ctx.TechnicalSpecs["тип_конструкции"] = "многослойная"
					ctx.TechnicalSpecs["назначение"] = "строительные_ограждающие_конструкции"
					return ctx
				}
			}
		}
	}

	// Дополнительная логика определения типа
	switch {
	case strings.Contains(name, "сэндвич") || strings.Contains(name, "isowall") || strings.Contains(name, "sandwich"):
		ctx.ProductType = "сэндвич_панель"
		ctx.TechnicalSpecs["тип_конструкции"] = "многослойная"
		ctx.TechnicalSpecs["назначение"] = "строительные_ограждающие_конструкции"
	case strings.Contains(name, "панель") && strings.Contains(name, "стен"):
		ctx.ProductType = "стеновая_панель"
	case strings.Contains(name, "панель") && strings.Contains(name, "кров"):
		ctx.ProductType = "кровельная_панель"
	}

	return ctx
}

// addTechnicalSpecs добавляет технические характеристики
func (ce *ContextEnricher) addTechnicalSpecs(ctx EnrichedContext) EnrichedContext {
	if ctx.ProductType == "сэндвич_панель" {
		ctx.TechnicalSpecs["материал_обшивки"] = "металл"
		ctx.TechnicalSpecs["наполнитель"] = "минеральная_вата"
		ctx.TechnicalSpecs["огнестойкость"] = "высокая"
		ctx.TechnicalSpecs["теплоизоляция"] = "высокая"
		ctx.TechnicalSpecs["звукоизоляция"] = "высокая"
		ctx.Confidence = 0.9
	}
	return ctx
}

// extractMainKeywords извлекает основные ключевые слова из названия
func extractMainKeywords(name string) string {
	words := strings.Fields(name)
	if len(words) > 3 {
		return words[0] // возвращаем первое слово как основной ключевой термин
	}
	return name
}

// BuildEnhancedDescription строит расширенное описание для классификатора
func (ctx *EnrichedContext) BuildEnhancedDescription(originalDescription string) string {
	var sb strings.Builder

	// Основное название
	sb.WriteString("Товар: ")
	sb.WriteString(ctx.NormalizedName)
	sb.WriteString("\n\n")

	// Тип продукта
	if ctx.ProductType != "" {
		sb.WriteString("Тип изделия: ")
		sb.WriteString(ctx.ProductType)
		sb.WriteString("\n")
	}

	// Ключевые характеристики
	if len(ctx.TechnicalSpecs) > 0 {
		sb.WriteString("Характеристики: ")
		first := true
		for key, value := range ctx.TechnicalSpecs {
			if !first {
				sb.WriteString(", ")
			}
			sb.WriteString(key)
			sb.WriteString(" - ")
			sb.WriteString(value)
			first = false
		}
		sb.WriteString("\n")
	}

	// Ключевые слова
	if len(ctx.Keywords) > 0 {
		sb.WriteString("Свойства: ")
		sb.WriteString(strings.Join(ctx.Keywords, ", "))
		sb.WriteString("\n")
	}

	// Описание из базы (если есть)
	if ctx.Description != "" {
		sb.WriteString("Описание: ")
		sb.WriteString(ctx.Description)
		sb.WriteString("\n")
	}

	// Исходное описание
	if originalDescription != "" {
		sb.WriteString("Дополнительно: ")
		sb.WriteString(originalDescription)
	}

	return sb.String()
}

// ClearCache очищает кэш обогащенного контекста
func (ce *ContextEnricher) ClearCache() {
	ce.mu.Lock()
	defer ce.mu.Unlock()
	ce.cache = make(map[string]EnrichedContext)
}

