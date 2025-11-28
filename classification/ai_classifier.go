package classification

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"httpserver/nomenclature"
)

// AIClassifier классификатор категорий с использованием AI
type AIClassifier struct {
	aiClient       *nomenclature.AIClient
	classifierTree *CategoryNode
}

// Reuse CategoryNode from classifier.go
// AIClassificationRequest запрос на классификацию
type AIClassificationRequest struct {
	ItemName    string                 `json:"item_name"`
	Description string                 `json:"description,omitempty"`
	Context     map[string]interface{} `json:"context,omitempty"`
	MaxLevels   int                    `json:"max_levels,omitempty"`
}

// AIClassificationResponse ответ от AI классификатора
type AIClassificationResponse struct {
	CategoryPath []string   `json:"category_path"`
	Confidence   float64    `json:"confidence"`
	Reasoning    string     `json:"reasoning"`
	Alternatives [][]string `json:"alternatives,omitempty"`
}

// NewAIClassifier создает новый AI классификатор
func NewAIClassifier(apiKey string, model string) *AIClassifier {
	if model == "" {
		// Пытаемся получить модель из переменной окружения
		model = os.Getenv("ARLIAI_MODEL")
		if model == "" {
			model = "GLM-4.5-Air" // Последний fallback
		}
	}
	return &AIClassifier{
		aiClient: nomenclature.NewAIClient(apiKey, model),
	}
}

// SetClassifierTree устанавливает дерево классификатора
func (ai *AIClassifier) SetClassifierTree(tree *CategoryNode) {
	ai.classifierTree = tree
}

// ClassifyWithAI определяет категорию товара с помощью AI
func (ai *AIClassifier) ClassifyWithAI(request AIClassificationRequest) (*AIClassificationResponse, error) {
	// Подготавливаем промпт
	prompt := ai.buildClassificationPrompt(request)

	// Вызываем AI
	response, err := ai.callAI(prompt)
	if err != nil {
		return nil, fmt.Errorf("AI request failed: %w", err)
	}

	// Парсим ответ
	return ai.parseAIResponse(response)
}

// buildClassificationPrompt строит промпт для классификации
func (ai *AIClassifier) buildClassificationPrompt(request AIClassificationRequest) string {
	classifierSummary := ai.summarizeClassifierTree()

	return fmt.Sprintf(`Ты - эксперт по классификации товаров и услуг.

ТВОЯ ЗАДАЧА:
Определить наиболее подходящий путь категории для объекта из предложенного классификатора.

ОБЪЕКТ:
Название: %s
Описание: %s

КЛАССИФИКАТОР КАТЕГОРИЙ:
%s

ОСНОВНЫЕ ПРИНЦИПЫ КЛАССИФИКАЦИИ:

1. РАЗГРАНИЧЕНИЕ ТОВАР/УСЛУГА:
   - ТОВАРЫ: физические объекты, материалы, оборудование, изделия, комплектующие
   - УСЛУГИ: работы, действия, консультации, техническое обслуживание, выполнение работ

2. КРИТИЧЕСКИЕ ПРИЗНАКИ ТОВАРА:
   - Наличие физической формы и материальной сущности
   - Возможность поставки, хранения, инвентаризации
   - Указание конкретных характеристик (размеры, марки, модели, артикулы)
   - Наличие технических параметров (диаметр, длина, вес, материал)

3. ПРАВИЛА ВЫБОРА КАТЕГОРИИ:
   - Выбирай наиболее специфичный (детальный) путь
   - Путь должен быть полным (от корня до листа)
   - Если не уверен - используй более общий путь
   - Учитывай назначение и материал объекта

4. ТИПИЧНЫЕ ОШИБКИ (ИЗБЕГАТЬ):
   - НЕ классифицировать оборудование/приборы как услуги по испытаниям
   - НЕ классифицировать материалы/комплектующие как прочие услуги
   - НЕ классифицировать кабели как печатные платы
   - НЕ классифицировать строительные элементы как прочие изделия
   - НЕ классифицировать датчики/преобразователи как услуги
   - НЕ классифицировать сэндвич-панели (isowall, sandwich panel) как изделия из гипса/бетона

5. ПРАВИЛА КЛАССИФИКАЦИИ СТРОИТЕЛЬНЫХ МАТЕРИАЛОВ:
   - СЭНДВИЧ-ПАНЕЛИ (металлическая обшивка + утеплитель):
     * Содержат "isowall", "сэндвич", "sandwich", "isopan" → 25.11.1 (Металлические конструкции)
     * НЕ относятся к 23.69.19 (Изделия из гипса, бетона или цемента)
     * Это многослойные конструкции с металлической обшивкой и наполнителем
   - ИЗДЕЛИЯ ИЗ МИНЕРАЛЬНЫХ МАТЕРИАЛОВ:
     * Только если основной материал гипс/бетон/цемент
     * Сэндвич-панели с минеральной ватой НЕ относятся сюда

6. ПРИМЕРЫ ПРАВИЛЬНОЙ КЛАССИФИКАЦИИ:
   - "фасонные элементы для панелей" → строительные конструкции, НЕ услуги
   - "преобразователь давления" → измерительные приборы, НЕ услуги по испытаниям
   - "контрольный кабель" → кабели, НЕ печатные платы
   - "датчик давления" → измерительные приборы, НЕ услуги
   - "панель isowall box" → металлические конструкции (25.11.1), НЕ изделия из гипса (23.69.19)
   - "сэндвич панель" → металлические конструкции (25.11.1), НЕ изделия из гипса (23.69.19)

КОГДА ВИДИШЬ ТОВАР (физический объект):
- Исключи категории услуг (разделы 33-99, особенно 71.20.1, 96.09.1)
- Найди наиболее специфическую категорию оборудования/материалов
- Обрати внимание на технические характеристики (марки, модели, размеры)
- Если видишь марку/модель (например: AKS, HELUKABEL, MQ) - это точно товар
- Если видишь технические параметры (давление, диаметр, длина) - это товар

КОГДА ВИДИШЬ УСЛУГУ (действие, работу):
- Убедись, что это действительно услуга, а не описание товара
- Проверь наличие признаков выполнения работ (монтаж, установка, ремонт, испытание)
- Если в названии есть марка/модель товара - это НЕ услуга, а товар

ФОРМАТ ОТВЕТА - ТОЛЬКО JSON:
{
    "category_path": ["Уровень1", "Уровень2", "Уровень3"],
    "confidence": 0.95,
    "reasoning": "Краткое обоснование выбора",
    "alternatives": [["Альтернативный", "Путь"]]
}

ВАЖНО:
- Отвечай ТОЛЬКО в указанном JSON формате
- Не добавляй никакого текста кроме JSON
- Убедись что путь существует в классификаторе
- Проверь соответствие выбранного пути физическим характеристикам объекта`,
		request.ItemName,
		request.Description,
		classifierSummary,
	)
}

// summarizeClassifierTree создает текстовое представление классификатора для AI
func (ai *AIClassifier) summarizeClassifierTree() string {
	if ai.classifierTree == nil {
		return "Классификатор не загружен"
	}

	// Ограничиваем глубину для экономии токенов (макс 3 уровня)
	return ai.traverseTreeSummary(ai.classifierTree, 0, 3)
}

// traverseTreeSummary обходит дерево и создает текстовое представление
func (ai *AIClassifier) traverseTreeSummary(node *CategoryNode, currentLevel, maxLevel int) string {
	if node == nil || currentLevel >= maxLevel {
		return ""
	}

	var result strings.Builder
	indent := strings.Repeat("  ", currentLevel)
	result.WriteString(fmt.Sprintf("%s- %s (ID: %s)\n", indent, node.Name, node.ID))

	// Рекурсивно обходим детей
	for i := range node.Children {
		childSummary := ai.traverseTreeSummary(&node.Children[i], currentLevel+1, maxLevel)
		if childSummary != "" {
			result.WriteString(childSummary)
		}
	}

	return result.String()
}

// callAI вызывает AI API
func (ai *AIClassifier) callAI(prompt string) (string, error) {
	// Используем GetCompletion из AIClient
	// Но сначала нужно проверить, есть ли такой метод
	// Если нет, используем ProcessProduct с кастомным промптом

	// Создаем системный промпт
	systemPrompt := `Ты - эксперт по классификации товаров и услуг. 

ВАЖНО: 
- Физические товары (материалы, оборудование, изделия) НЕ могут быть услугами
- Если видишь марку, модель, технические характеристики - это товар
- Услуги описывают действия, работы, консультации, а не физические объекты

Отвечай только в формате JSON.`

	// Используем ProcessProduct, но нам нужен другой формат ответа
	// Создадим временный метод или используем существующий

	// Для простоты используем прямой вызов через GetCompletion если доступен
	// Иначе используем ProcessProduct и парсим ответ

	// Пока используем упрощенный подход - вызываем через стандартный метод
	result, err := ai.aiClient.GetCompletion(systemPrompt, prompt)
	if err != nil {
		return "", fmt.Errorf("AI API call failed: %w", err)
	}

	return result, nil
}

// parseAIResponse парсит ответ от AI
func (ai *AIClassifier) parseAIResponse(response string) (*AIClassificationResponse, error) {
	// Очищаем ответ от возможных markdown блоков
	response = strings.TrimSpace(response)
	if strings.HasPrefix(response, "```json") {
		response = strings.TrimPrefix(response, "```json")
		response = strings.TrimSuffix(response, "```")
		response = strings.TrimSpace(response)
	} else if strings.HasPrefix(response, "```") {
		response = strings.TrimPrefix(response, "```")
		response = strings.TrimSuffix(response, "```")
		response = strings.TrimSpace(response)
	}

	var aiResponse AIClassificationResponse
	if err := json.Unmarshal([]byte(response), &aiResponse); err != nil {
		return nil, fmt.Errorf("failed to parse AI response: %w, response: %s", err, response)
	}

	// Валидация
	if len(aiResponse.CategoryPath) == 0 {
		return nil, fmt.Errorf("empty category path in AI response")
	}

	if aiResponse.Confidence <= 0 || aiResponse.Confidence > 1 {
		aiResponse.Confidence = 0.7 // Дефолтная уверенность
	}

	return &aiResponse, nil
}

// CodeExists проверяет существование пути в классификаторе
func (ai *AIClassifier) CodeExists(path []string) bool {
	if ai.classifierTree == nil {
		return false
	}

	// Проверяем путь в дереве
	current := ai.classifierTree
	for _, levelName := range path {
		found := false
		for i := range current.Children {
			if current.Children[i].Name == levelName {
				current = &current.Children[i]
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}
