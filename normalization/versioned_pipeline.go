package normalization

import (
	"encoding/json"
	"fmt"

	"httpserver/database"
)

// VersionedNormalizationPipeline управляет многостадийной нормализацией с версионированием
type VersionedNormalizationPipeline struct {
	db              *database.DB
	sessionID       int
	catalogItemID   int
	originalName    string
	currentName     string
	stages          []*database.NormalizationStage
	patternDetector *PatternDetector
	aiIntegrator    *PatternAIIntegrator
	metadata        map[string]interface{}
}

// GetDB возвращает указатель на базу данных (для доступа из других пакетов)
func (p *VersionedNormalizationPipeline) GetDB() *database.DB {
	return p.db
}

// NewVersionedNormalizationPipeline создает новый версионированный пайплайн
func NewVersionedNormalizationPipeline(
	db *database.DB,
	patternDetector *PatternDetector,
	aiIntegrator *PatternAIIntegrator,
) *VersionedNormalizationPipeline {
	return &VersionedNormalizationPipeline{
		db:              db,
		patternDetector: patternDetector,
		aiIntegrator:    aiIntegrator,
		metadata:        make(map[string]interface{}),
		stages:          make([]*database.NormalizationStage, 0),
	}
}

// StartSession начинает новую сессию нормализации
func (p *VersionedNormalizationPipeline) StartSession(catalogItemID int, originalName string) error {
	sessionID, err := p.db.CreateNormalizationSession(catalogItemID, originalName)
	if err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	p.sessionID = sessionID
	p.catalogItemID = catalogItemID
	p.originalName = originalName
	p.currentName = originalName
	p.stages = make([]*database.NormalizationStage, 0)

	return nil
}

// ApplyPatterns применяет алгоритмические паттерны для исправления названия
func (p *VersionedNormalizationPipeline) ApplyPatterns() error {
	if p.sessionID == 0 {
		return fmt.Errorf("session not started")
	}

	// Обнаруживаем паттерны
	matches := p.patternDetector.DetectPatterns(p.currentName)

	// Применяем исправления
	fixedName := p.patternDetector.ApplyFixes(p.currentName, matches)

	// Сериализуем паттерны в JSON
	patternsJSON, err := json.Marshal(matches)
	if err != nil {
		patternsJSON = []byte("[]")
	}

	// Вычисляем уверенность на основе паттернов
	confidence := p.calculatePatternConfidence(matches)

	// Создаем стадию
	stage := &database.NormalizationStage{
		SessionID:       p.sessionID,
		StageType:       "algorithmic",
		StageName:       "pattern_correction",
		InputName:       p.currentName,
		OutputName:      fixedName,
		AppliedPatterns: string(patternsJSON),
		Confidence:      confidence,
		Status:          "applied",
	}

	// Сохраняем стадию
	if err := p.db.AddNormalizationStage(stage); err != nil {
		return fmt.Errorf("failed to save stage: %w", err)
	}

	// Обновляем текущее имя
	p.currentName = fixedName
	p.stages = append(p.stages, stage)

	return nil
}

// ApplyAICorrection применяет AI коррекцию
func (p *VersionedNormalizationPipeline) ApplyAICorrection(useChat bool, context ...string) error {
	if p.sessionID == 0 {
		return fmt.Errorf("session not started")
	}

	if p.aiIntegrator == nil {
		return fmt.Errorf("AI integrator not available")
	}

	// Получаем историю для чат-контекста, если нужно
	var aiContext map[string]interface{}
	if useChat {
		aiContext = p.buildChatContext()
	}

	// Получаем предложение от AI
	result, err := p.aiIntegrator.SuggestCorrectionWithAI(p.currentName)
	if err != nil {
		return fmt.Errorf("AI correction failed: %w", err)
	}

	// Формируем AI контекст для сохранения
	aiContextJSON, err := json.Marshal(map[string]interface{}{
		"use_chat":        useChat,
		"context":         context,
		"chat_history":    aiContext,
		"ai_result":       result,
		"original_name":   p.currentName,
		"suggested_name":  result.FinalSuggestion,
		"confidence":      result.Confidence,
		"reasoning":       result.Reasoning,
	})
	if err != nil {
		aiContextJSON = []byte("{}")
	}

	// Определяем тип стадии
	stageType := "ai_single"
	if useChat {
		stageType = "ai_chat"
	}

	// Создаем стадию
	stage := &database.NormalizationStage{
		SessionID:       p.sessionID,
		StageType:       stageType,
		StageName:       "ai_correction",
		InputName:       p.currentName,
		OutputName:      result.FinalSuggestion,
		AIContext:        string(aiContextJSON),
		Confidence:      result.Confidence,
		Status:          "applied",
	}

	// Сохраняем стадию
	if err := p.db.AddNormalizationStage(stage); err != nil {
		return fmt.Errorf("failed to save stage: %w", err)
	}

	// Обновляем текущее имя
	p.currentName = result.FinalSuggestion
	p.stages = append(p.stages, stage)

	return nil
}

// GetHistory получает полную историю стадий
func (p *VersionedNormalizationPipeline) GetHistory() ([]*database.NormalizationStage, error) {
	if p.sessionID == 0 {
		return nil, fmt.Errorf("session not started")
	}

	return p.db.GetSessionHistory(p.sessionID)
}

// RevertToStage откатывает к указанной стадии
func (p *VersionedNormalizationPipeline) RevertToStage(targetStageID int) error {
	if p.sessionID == 0 {
		return fmt.Errorf("session not started")
	}

	// Откатываем в БД
	if err := p.db.RevertToStage(p.sessionID, targetStageID); err != nil {
		return fmt.Errorf("failed to revert: %w", err)
	}

	// Обновляем локальное состояние
	history, err := p.GetHistory()
	if err != nil {
		return fmt.Errorf("failed to get history: %w", err)
	}

	p.stages = history
	if len(history) > 0 {
		p.currentName = history[len(history)-1].OutputName
	} else {
		p.currentName = p.originalName
	}

	return nil
}

// GetCurrentName возвращает текущее имя после всех примененных стадий
func (p *VersionedNormalizationPipeline) GetCurrentName() string {
	return p.currentName
}

// GetSessionID возвращает ID сессии
func (p *VersionedNormalizationPipeline) GetSessionID() int {
	return p.sessionID
}

// SetMetadata устанавливает метаданные
func (p *VersionedNormalizationPipeline) SetMetadata(key string, value interface{}) {
	if p.metadata == nil {
		p.metadata = make(map[string]interface{})
	}
	p.metadata[key] = value
}

// GetMetadata получает метаданные
func (p *VersionedNormalizationPipeline) GetMetadata(key string) interface{} {
	if p.metadata == nil {
		return nil
	}
	return p.metadata[key]
}

// CompleteSession завершает сессию
func (p *VersionedNormalizationPipeline) CompleteSession() error {
	if p.sessionID == 0 {
		return fmt.Errorf("session not started")
	}

	return p.db.UpdateSessionStatus(p.sessionID, "completed")
}

// buildChatContext строит контекст для чат-подхода на основе истории стадий
func (p *VersionedNormalizationPipeline) buildChatContext() map[string]interface{} {
	context := make(map[string]interface{})
	
	// Собираем историю предыдущих AI стадий
	var chatHistory []map[string]interface{}
	for _, stage := range p.stages {
		if stage.StageType == "ai_single" || stage.StageType == "ai_chat" {
			chatHistory = append(chatHistory, map[string]interface{}{
				"input":   stage.InputName,
				"output":  stage.OutputName,
				"stage":   stage.StageName,
				"confidence": stage.Confidence,
			})
		}
	}

	context["history"] = chatHistory
	context["stages_count"] = len(p.stages)
	context["original_name"] = p.originalName

	return context
}

// calculatePatternConfidence вычисляет уверенность на основе найденных паттернов
func (p *VersionedNormalizationPipeline) calculatePatternConfidence(matches []PatternMatch) float64 {
	if len(matches) == 0 {
		return 1.0 // Нет паттернов - высокая уверенность
	}

	totalConfidence := 0.0
	for _, match := range matches {
		if match.AutoFixable {
			totalConfidence += match.Confidence
		} else {
			totalConfidence += match.Confidence * 0.7 // Снижаем для неавтоприменяемых
		}
	}

	avgConfidence := totalConfidence / float64(len(matches))
	
	// Если все паттерны автоприменяемые, уверенность выше
	allAutoFixable := true
	for _, match := range matches {
		if !match.AutoFixable {
			allAutoFixable = false
			break
		}
	}

	if allAutoFixable {
		return avgConfidence * 0.95
	}

	return avgConfidence * 0.8
}

