package normalization

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"httpserver/database"
	"httpserver/nomenclature"
)

// KpvedClassificationResult результат классификации по КПВЭД
type KpvedClassificationResult struct {
	KpvedCode       string  `json:"kpved_code"`
	KpvedName       string  `json:"kpved_name"`
	KpvedConfidence float64 `json:"kpved_confidence"`
	Reasoning       string  `json:"reasoning"`
}

// KpvedClassifier классификатор КПВЭД для нормализации
type KpvedClassifier struct {
	db             *database.DB
	kpvedProcessor *nomenclature.KpvedProcessor
	aiClient       *nomenclature.AIClient
}

// NewKpvedClassifier создает новый классификатор КПВЭД
// Если model пустая, используется значение из переменной окружения ARLIAI_MODEL или дефолт "GLM-4.5-Air"
func NewKpvedClassifier(db *database.DB, apiKey string, kpvedFilePath string, model ...string) *KpvedClassifier {
	modelName := "GLM-4.5-Air" // Дефолтная модель
	if len(model) > 0 && model[0] != "" {
		modelName = model[0]
	} else {
		// Пытаемся получить из переменной окружения
		if envModel := os.Getenv("ARLIAI_MODEL"); envModel != "" {
			modelName = envModel
		}
	}
	classifier := &KpvedClassifier{
		db:             db,
		kpvedProcessor: nomenclature.NewKpvedProcessor(),
		aiClient:       nomenclature.NewAIClient(apiKey, modelName),
	}

	// Загружаем справочник КПВЭД для AI промптов
	if err := classifier.kpvedProcessor.LoadKpved(kpvedFilePath); err != nil {
		log.Printf("Warning: Failed to load KPVED file for classifier: %v", err)
	}

	return classifier
}

// ClassifyWithKpved классифицирует товар по КПВЭД используя AI
func (k *KpvedClassifier) ClassifyWithKpved(normalizedName string) (*KpvedClassificationResult, error) {
	if k.aiClient == nil {
		return nil, fmt.Errorf("AI client not available")
	}

	// Подготавливаем промпт для AI
	prompt := k.buildClassificationPrompt(normalizedName)

	// Вызываем AI API
	response, err := k.callAIForClassification(prompt)
	if err != nil {
		return nil, fmt.Errorf("failed to call AI for KPVED classification: %w", err)
	}

	// Парсим ответ
	result, err := k.parseAIResponse(response)
	if err != nil {
		return nil, fmt.Errorf("failed to parse AI response: %w", err)
	}

	// Валидируем код КПВЭД в базе данных
	if err := k.validateKpvedCode(result); err != nil {
		log.Printf("Warning: KPVED code validation failed for %s: %v", result.KpvedCode, err)
		// Не возвращаем ошибку, просто уменьшаем confidence
		result.KpvedConfidence *= 0.5
	}

	return result, nil
}

// buildClassificationPrompt создает промпт для AI классификации
func (k *KpvedClassifier) buildClassificationPrompt(normalizedName string) string {
	kpvedData := k.kpvedProcessor.GetData()

	// Ограничиваем размер справочника для промпта (берем только релевантные части)
	// Для полного функционала можно использовать весь справочник
	kpvedSample := k.getRelevantKpvedSample(normalizedName, kpvedData)

	prompt := fmt.Sprintf(`Ты - эксперт по классификации товаров по справочнику КПВЭД (Классификатор продукции по видам экономической деятельности).

Задача: Определить код КПВЭД для следующего нормализованного названия товара: "%s"

Справочник КПВЭД (фрагмент):
%s

Инструкции:
1. Найди наиболее подходящий код КПВЭД для этого товара
2. Код должен быть максимально конкретным (чем больше уровней, тем лучше)
3. Верни результат в формате JSON:
{
  "kpved_code": "XX.YY.ZZ",
  "kpved_name": "Название из справочника",
  "kpved_confidence": 0.95,
  "reasoning": "Краткое объяснение почему выбран этот код"
}

Важно:
- kpved_code должен точно соответствовать коду из справочника
- kpved_confidence должен быть от 0 до 1
- Если не уверен, выбери более общий код (с меньшим количеством уровней)
- Если товар не подходит ни под одну категорию, верни код "99" с confidence 0.3

Ответ (только JSON, без дополнительного текста):`, normalizedName, kpvedSample)

	return prompt
}

// getRelevantKpvedSample получает релевантный фрагмент КПВЭД для промпта
func (k *KpvedClassifier) getRelevantKpvedSample(normalizedName string, fullData string) string {
	// Для упрощения возвращаем весь справочник
	// В production можно оптимизировать, выбирая только релевантные разделы
	// на основе ключевых слов из normalizedName

	// Ограничиваем размер (берем первые 50000 символов)
	if len(fullData) > 50000 {
		return fullData[:50000] + "\n... (справочник сокращен)"
	}

	return fullData
}

// callAIForClassification вызывает AI API для классификации
func (k *KpvedClassifier) callAIForClassification(userPrompt string) (string, error) {
	// Используем AIClient для вызова API
	systemPrompt := "Ты - эксперт по классификации товаров по справочнику КПВЭД."

	// Вызываем AI через клиент
	response, err := k.aiClient.GetCompletion(systemPrompt, userPrompt)
	if err != nil {
		return "", fmt.Errorf("failed to call AI API: %w", err)
	}

	return response, nil
}

// parseAIResponse парсит ответ AI и извлекает результат классификации
func (k *KpvedClassifier) parseAIResponse(response string) (*KpvedClassificationResult, error) {
	// Очищаем ответ от markdown кода если есть
	response = strings.TrimSpace(response)
	response = strings.TrimPrefix(response, "```json")
	response = strings.TrimPrefix(response, "```")
	response = strings.TrimSuffix(response, "```")
	response = strings.TrimSpace(response)

	var result KpvedClassificationResult
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal AI response: %w", err)
	}

	// Проверяем обязательные поля
	if result.KpvedCode == "" {
		return nil, fmt.Errorf("kpved_code is empty in AI response")
	}

	// Устанавливаем значения по умолчанию
	if result.KpvedConfidence == 0 {
		result.KpvedConfidence = 0.5
	}
	if result.KpvedName == "" {
		result.KpvedName = "Не определено"
	}

	return &result, nil
}

// validateKpvedCode проверяет что код КПВЭД существует в базе данных
func (k *KpvedClassifier) validateKpvedCode(result *KpvedClassificationResult) error {
	query := `SELECT name FROM kpved_classifier WHERE code = ?`

	var dbName string
	err := k.db.QueryRow(query, result.KpvedCode).Scan(&dbName)
	if err != nil {
		if err == sql.ErrNoRows {
			return fmt.Errorf("KPVED code %s not found in database", result.KpvedCode)
		}
		return fmt.Errorf("database error: %w", err)
	}

	// Обновляем название из базы (оно более надежное чем от AI)
	result.KpvedName = dbName

	return nil
}

// GetKpvedByCode получает информацию о коде КПВЭД из базы данных
func (k *KpvedClassifier) GetKpvedByCode(code string) (string, error) {
	query := `SELECT name FROM kpved_classifier WHERE code = ?`

	var name string
	err := k.db.QueryRow(query, code).Scan(&name)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("KPVED code %s not found", code)
		}
		return "", fmt.Errorf("database error: %w", err)
	}

	return name, nil
}

// SearchKpvedByName ищет коды КПВЭД по названию
func (k *KpvedClassifier) SearchKpvedByName(searchTerm string, limit int) ([]map[string]interface{}, error) {
	query := `
		SELECT code, name, parent_code, level
		FROM kpved_classifier
		WHERE name LIKE ?
		ORDER BY level, code
		LIMIT ?
	`

	rows, err := k.db.Query(query, "%"+searchTerm+"%", limit)
	if err != nil {
		return nil, fmt.Errorf("failed to search KPVED: %w", err)
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var code, name string
		var parentCode sql.NullString
		var level int

		if err := rows.Scan(&code, &name, &parentCode, &level); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		result := map[string]interface{}{
			"code":   code,
			"name":   name,
			"level":  level,
		}

		if parentCode.Valid {
			result["parent_code"] = parentCode.String
		}

		results = append(results, result)
	}

	return results, nil
}
