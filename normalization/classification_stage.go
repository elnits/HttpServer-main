package normalization

import (
	"encoding/json"
	"fmt"

	"httpserver/classification"
	"httpserver/database"
)

// ClassificationStage стадия классификации товара
type ClassificationStage struct {
	classifier *classification.AIClassifier
	strategies *classification.StrategyManager
}

// NewClassificationStage создает новую стадию классификации
func NewClassificationStage(
	classifier *classification.AIClassifier,
	strategies *classification.StrategyManager,
) *ClassificationStage {
	return &ClassificationStage{
		classifier: classifier,
		strategies: strategies,
	}
}

// Process выполняет классификацию и свертку категорий
func (cs *ClassificationStage) Process(
	pipeline *VersionedNormalizationPipeline,
	strategyID string,
) error {
	if pipeline.GetSessionID() == 0 {
		return fmt.Errorf("session not started")
	}

	// Определяем категорию с помощью AI
	aiRequest := classification.AIClassificationRequest{
		ItemName:    pipeline.GetCurrentName(),
		Description: getStringFromMetadata(pipeline.GetMetadata("description")),
		MaxLevels:   6, // Полная глубина классификатора
	}

	aiResponse, err := cs.classifier.ClassifyWithAI(aiRequest)
	if err != nil {
		return fmt.Errorf("AI classification failed: %w", err)
	}

	// Сворачиваем категорию до допустимой глубины
	foldedPath, err := cs.strategies.FoldCategory(aiResponse.CategoryPath, strategyID)
	if err != nil {
		// Если стратегия не найдена, используем простую свертку
		foldedPath = classification.FoldCategoryPathSimple(aiResponse.CategoryPath, 2, "top")
	}

	// Сериализуем категории в JSON
	originalJSON, _ := json.Marshal(aiResponse.CategoryPath)
	foldedJSON, _ := json.Marshal(foldedPath)

	// Сохраняем категорию в метаданные пайплайна
	pipeline.SetMetadata("category_original", aiResponse.CategoryPath)
	pipeline.SetMetadata("category_folded", foldedPath)
	pipeline.SetMetadata("classification_confidence", aiResponse.Confidence)
	pipeline.SetMetadata("classification_strategy", strategyID)

	// Создаем стадию нормализации
	stage := &database.NormalizationStage{
		SessionID:              pipeline.GetSessionID(),
		StageType:              "classification",
		StageName:              "ai_category_folding",
		InputName:              pipeline.GetCurrentName(),
		OutputName:             pipeline.GetCurrentName(), // Название не меняется
		CategoryOriginal:       string(originalJSON),
		CategoryFolded:         string(foldedJSON),
		ClassificationStrategy: strategyID,
		Confidence:             aiResponse.Confidence,
		Status:                 "applied",
	}

	// Сохраняем стадию через метод пайплайна
	// Нужно добавить метод AddStage в VersionedNormalizationPipeline
	// Пока используем прямой доступ к БД
	if err := pipeline.GetDB().AddNormalizationStage(stage); err != nil {
		return fmt.Errorf("failed to save classification stage: %w", err)
	}

	return nil
}

// getStringFromMetadata безопасно извлекает строку из метаданных
func getStringFromMetadata(value interface{}) string {
	if value == nil {
		return ""
	}
	if str, ok := value.(string); ok {
		return str
	}
	return ""
}

