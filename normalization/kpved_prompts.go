package normalization

import (
	"fmt"
	"strings"
)

// ClassificationPrompt промпт для классификации
type ClassificationPrompt struct {
	System string
	User   string
}

// PromptBuilder строитель промптов для классификации
type PromptBuilder struct {
	tree *KpvedTree
}

// NewPromptBuilder создает новый строитель промптов
func NewPromptBuilder(tree *KpvedTree) *PromptBuilder {
	return &PromptBuilder{tree: tree}
}

// BuildLevelPrompt строит промпт для указанного уровня
func (pb *PromptBuilder) BuildLevelPrompt(
	normalizedName string,
	category string,
	level KpvedLevel,
	candidates []*KpvedNode,
) *ClassificationPrompt {
	return pb.BuildLevelPromptWithType(normalizedName, category, level, candidates, "")
}

// BuildLevelPromptWithType строит промпт для указанного уровня с учетом типа объекта
func (pb *PromptBuilder) BuildLevelPromptWithType(
	normalizedName string,
	category string,
	level KpvedLevel,
	candidates []*KpvedNode,
	objectType string, // "product", "service", или ""
) *ClassificationPrompt {
	switch level {
	case LevelSection:
		return pb.buildSectionPrompt(normalizedName, category, candidates, objectType)
	case LevelClass:
		return pb.buildClassPrompt(normalizedName, category, candidates, objectType)
	case LevelSubclass:
		return pb.buildSubclassPrompt(normalizedName, category, candidates, objectType)
	case LevelGroup:
		return pb.buildGroupPrompt(normalizedName, category, candidates, objectType)
	default:
		return pb.buildSectionPrompt(normalizedName, category, candidates, objectType)
	}
}

// buildSectionPrompt строит промпт для уровня секций
func (pb *PromptBuilder) buildSectionPrompt(
	normalizedName string,
	category string,
	candidates []*KpvedNode,
	objectType string,
) *ClassificationPrompt {
	// Формируем список секций
	var sectionsText strings.Builder
	for _, candidate := range candidates {
		sectionsText.WriteString(fmt.Sprintf("- %s: %s\n", candidate.Code, candidate.Name))
	}

	// Добавляем правила разграничения товаров/услуг
	rulesText := pb.getClassificationRules(objectType)

	systemPrompt := fmt.Sprintf(`Ты - эксперт по классификации товаров и услуг по классификатору КПВЭД.

ОСНОВНЫЕ ПРИНЦИПЫ КЛАССИФИКАЦИИ:
%s

РАЗДЕЛЫ КПВЭД:
%s

ИНСТРУКЦИЯ:
1. Определи физическую природу объекта (товар или услуга)
2. Выбери наиболее подходящий раздел
3. Учитывай назначение и функциональные характеристики
4. Избегай типичных ошибок классификации

Ответь только JSON:
{
    "selected_code": "код раздела",
    "confidence": 0.95,
    "reasoning": "краткое объяснение выбора"
}`, rulesText, sectionsText.String())

	userPrompt := fmt.Sprintf("Объект: %s\nКатегория: %s", normalizedName, category)

	return &ClassificationPrompt{
		System: systemPrompt,
		User:   userPrompt,
	}
}

// buildClassPrompt строит промпт для уровня классов
func (pb *PromptBuilder) buildClassPrompt(
	normalizedName string,
	category string,
	candidates []*KpvedNode,
	objectType string,
) *ClassificationPrompt {
	if len(candidates) == 0 {
		return &ClassificationPrompt{}
	}

	// Получаем название секции
	parentCode := candidates[0].ParentCode
	parentNode, _ := pb.tree.GetNode(parentCode)
	sectionName := "неизвестный раздел"
	if parentNode != nil {
		sectionName = parentNode.Name
	}

	// Формируем список классов (ограничиваем до 30 для краткости)
	var classesText strings.Builder
	maxClasses := 30
	for i, candidate := range candidates {
		if i >= maxClasses {
			classesText.WriteString(fmt.Sprintf("... и еще %d классов\n", len(candidates)-maxClasses))
			break
		}
		classesText.WriteString(fmt.Sprintf("- %s: %s\n", candidate.Code, candidate.Name))
	}

	rulesText := pb.getClassificationRules(objectType)

	systemPrompt := fmt.Sprintf(`Выбери класс в разделе "%s".

ПРАВИЛА КЛАССИФИКАЦИИ:
%s

КЛАССЫ:
%s

Ответь только JSON:
{
    "selected_code": "код класса",
    "confidence": 0.90,
    "reasoning": "объяснение выбора"
}`, sectionName, rulesText, classesText.String())

	userPrompt := fmt.Sprintf("Объект: %s\nКатегория: %s", normalizedName, category)

	return &ClassificationPrompt{
		System: systemPrompt,
		User:   userPrompt,
	}
}

// buildSubclassPrompt строит промпт для уровня подклассов
func (pb *PromptBuilder) buildSubclassPrompt(
	normalizedName string,
	category string,
	candidates []*KpvedNode,
	objectType string,
) *ClassificationPrompt {
	if len(candidates) == 0 {
		return &ClassificationPrompt{}
	}

	// Получаем название класса
	parentCode := candidates[0].ParentCode
	parentNode, _ := pb.tree.GetNode(parentCode)
	className := "неизвестный класс"
	if parentNode != nil {
		className = parentNode.Name
	}

	// Формируем список подклассов (ограничиваем до 25)
	var subclassesText strings.Builder
	maxSubclasses := 25
	for i, candidate := range candidates {
		if i >= maxSubclasses {
			subclassesText.WriteString(fmt.Sprintf("... и еще %d подклассов\n", len(candidates)-maxSubclasses))
			break
		}
		subclassesText.WriteString(fmt.Sprintf("- %s: %s\n", candidate.Code, candidate.Name))
	}

	rulesText := pb.getClassificationRules(objectType)

	systemPrompt := fmt.Sprintf(`Выбери подкласс в классе "%s".

ПРАВИЛА КЛАССИФИКАЦИИ:
%s

ПОДКЛАССЫ:
%s

Ответь только JSON:
{
    "selected_code": "код подкласса",
    "confidence": 0.85,
    "reasoning": "объяснение выбора"
}`, className, rulesText, subclassesText.String())

	userPrompt := fmt.Sprintf("Объект: %s\nКатегория: %s", normalizedName, category)

	return &ClassificationPrompt{
		System: systemPrompt,
		User:   userPrompt,
	}
}

// buildGroupPrompt строит промпт для уровня групп
func (pb *PromptBuilder) buildGroupPrompt(
	normalizedName string,
	category string,
	candidates []*KpvedNode,
	objectType string,
) *ClassificationPrompt {
	if len(candidates) == 0 {
		return &ClassificationPrompt{}
	}

	// Получаем название подкласса
	parentCode := candidates[0].ParentCode
	parentNode, _ := pb.tree.GetNode(parentCode)
	subclassName := "неизвестный подкласс"
	if parentNode != nil {
		subclassName = parentNode.Name
	}

	// Формируем список групп (ограничиваем до 20)
	var groupsText strings.Builder
	maxGroups := 20
	for i, candidate := range candidates {
		if i >= maxGroups {
			groupsText.WriteString(fmt.Sprintf("... и еще %d групп\n", len(candidates)-maxGroups))
			break
		}
		groupsText.WriteString(fmt.Sprintf("- %s: %s\n", candidate.Code, candidate.Name))
	}

	rulesText := pb.getClassificationRules(objectType)

	systemPrompt := fmt.Sprintf(`Выбери группу в подклассе "%s".

ПРАВИЛА КЛАССИФИКАЦИИ:
%s

ГРУППЫ:
%s

Ответь только JSON:
{
    "selected_code": "код группы",
    "confidence": 0.80,
    "reasoning": "объяснение выбора"
}`, subclassName, rulesText, groupsText.String())

	userPrompt := fmt.Sprintf("Объект: %s\nКатегория: %s", normalizedName, category)

	return &ClassificationPrompt{
		System: systemPrompt,
		User:   userPrompt,
	}
}

// GetPromptSize возвращает примерный размер промпта в байтах
func (p *ClassificationPrompt) GetPromptSize() int {
	return len(p.System) + len(p.User)
}

// getClassificationRules возвращает универсальные правила классификации
func (pb *PromptBuilder) getClassificationRules(objectType string) string {
	rules := strings.Builder{}
	
	rules.WriteString("1. РАЗГРАНИЧЕНИЕ ТОВАР/УСЛУГА:\n")
	rules.WriteString("   - ТОВАРЫ: физические объекты, материалы, оборудование, изделия, комплектующие\n")
	rules.WriteString("   - УСЛУГИ: работы, действия, консультации, техническое обслуживание, испытания\n")
	rules.WriteString("   - КРИТИЧНО: если объект является товаром, НЕ выбирай категории услуг (разделы 33-99)\n\n")
	
	rules.WriteString("2. ПРИЗНАКИ ТОВАРА (физический объект):\n")
	rules.WriteString("   - Наличие марки, модели, артикула (например: AKS, HELUKABEL, MQ)\n")
	rules.WriteString("   - Указание размеров, технических характеристик (диаметр, длина, давление)\n")
	rules.WriteString("   - Названия материалов, компонентов, элементов (кабель, датчик, панель, элемент)\n")
	rules.WriteString("   - Возможность поставки, хранения, инвентаризации\n\n")
	
	rules.WriteString("3. ПРИЗНАКИ УСЛУГИ (действие, работа):\n")
	rules.WriteString("   - Описание действий (монтаж, установка, ремонт, испытание, консультация)\n")
	rules.WriteString("   - Упоминание работ, услуг, обслуживания\n")
	rules.WriteString("   - Отсутствие физических характеристик товара\n\n")
	
	rules.WriteString("4. ТИПИЧНЫЕ ОШИБКИ (ИЗБЕГАТЬ):\n")
	rules.WriteString("   - Классифицировать оборудование/датчики как услуги по испытаниям\n")
	rules.WriteString("   - Классифицировать материалы/компоненты как прочие услуги\n")
	rules.WriteString("   - Классифицировать кабели как электронные платы\n")
	rules.WriteString("   - Классифицировать строительные элементы как прочие изделия\n\n")
	
	rules.WriteString("5. ПРАВИЛА КЛАССИФИКАЦИИ СТРОИТЕЛЬНЫХ МАТЕРИАЛОВ:\n")
	rules.WriteString("   - СЭНДВИЧ-ПАНЕЛИ (металлическая обшивка + утеплитель):\n")
	rules.WriteString("     * Содержат \"isowall\", \"сэндвич\", \"sandwich\", \"isopan\" → 25.11.1 (Металлические конструкции)\n")
	rules.WriteString("     * НЕ относятся к 23.69.19 (Изделия из гипса, бетона или цемента)\n")
	rules.WriteString("     * Это многослойные конструкции с металлической обшивкой\n")
	rules.WriteString("   - ИЗДЕЛИЯ ИЗ МИНЕРАЛЬНЫХ МАТЕРИАЛОВ (гипс, бетон, цемент):\n")
	rules.WriteString("     * Только если основной материал гипс/бетон/цемент\n")
	rules.WriteString("     * Сэндвич-панели с минеральной ватой НЕ относятся сюда\n")
	rules.WriteString("   - КОНСТРУКЦИОННЫЕ ПАНЕЛИ:\n")
	rules.WriteString("     * Металлические конструкции → 25.11.1\n")
	rules.WriteString("     * Пластмассовые → 23.62.1\n")
	rules.WriteString("     * Из минеральных материалов → 23.69.19\n\n")
	
	rules.WriteString("6. ПРИМЕРЫ ПРАВИЛЬНОЙ КЛАССИФИКАЦИИ:\n")
	rules.WriteString("   - \"кабель контрольный helukabel\" → Кабели (27.32), НЕ Платы (26.12)\n")
	rules.WriteString("   - \"преобразователь давления aks\" → Приборы измерения (26.51), НЕ Услуги испытаний (71.20)\n")
	rules.WriteString("   - \"фасонные элементы для панелей\" → Строительные изделия (23.62/25.11), НЕ Услуги (96.09)\n")
	rules.WriteString("   - \"болт м10\" → Метизы (25.93), НЕ Услуги\n")
	rules.WriteString("   - \"панель isowall box\" → Металлические конструкции (25.11.1), НЕ Изделия из гипса (23.69.19)\n")
	rules.WriteString("   - \"сэндвич панель\" → Металлические конструкции (25.11.1), НЕ Изделия из гипса (23.69.19)\n\n")
	
	if objectType == "product" {
		rules.WriteString("ВАЖНО: Объект определен как ТОВАР. Исключи все категории услуг из рассмотрения.\n")
	} else if objectType == "service" {
		rules.WriteString("ВАЖНО: Объект определен как УСЛУГА. Выбирай только категории услуг.\n")
	}
	
	return rules.String()
}

// FormatForAPI форматирует промпт для отправки в API
func (p *ClassificationPrompt) FormatForAPI() map[string]string {
	return map[string]string{
		"system": p.System,
		"user":   p.User,
	}
}
